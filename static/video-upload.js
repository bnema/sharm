/**
 * @param {File} file
 * @returns {boolean}
 */
function isVideoFile(file) {
  return file.type.startsWith('video/') || /\.(mp4|webm|mov|avi|mkv|flv|wmv|m4v)$/i.test(file.name);
}

/**
 * The browser fast path is deliberately conservative. The server still
 * validates the actual container and codecs with ffprobe before publishing.
 * @param {File} file
 * @returns {'direct' | 'server-fallback'}
 */
function chooseVideoPreparationPath(file) {
  if (!isVideoFile(file)) return 'server-fallback';
  const canPlayH264 = document.createElement('video').canPlayType('video/mp4; codecs="avc1.4d401f, mp4a.40.2"');
  if (file.type === 'video/mp4' && canPlayH264 !== '') return 'direct';
  return 'server-fallback';
}

/**
 * @param {string} url
 * @param {RequestInit} [options]
 * @returns {Promise<any>}
 */
async function fetchUploadJSON(url, options) {
  const response = await fetch(url, options);
  const text = await response.text();
  let payload = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch (_) {
      payload = null;
    }
  }
  if (!response.ok) {
    const message = payload && typeof payload.error === 'string' ? payload.error : 'Upload request failed';
    const error = new Error(message);
    // @ts-ignore - status is attached for retry decisions
    error.status = response.status;
    throw error;
  }
  return payload;
}

/**
 * @param {Blob} blob
 * @returns {Promise<string>}
 */
async function hashBlob(blob) {
  if (!globalThis.crypto?.subtle) return '';
  const digest = await globalThis.crypto.subtle.digest('SHA-256', await blob.arrayBuffer());
  return bytesToHex(new Uint8Array(digest));
}

/** @param {unknown} error */
function isPermanentUploadError(error) {
  // @ts-ignore - status is attached by fetchUploadJSON
  const status = error && error.status;
  return status >= 400 && status < 500 && status !== 409;
}

/** @param {string} sessionID */
async function cancelServerUploadSession(sessionID) {
  try {
    await fetchUploadJSON('/upload/session/' + encodeURIComponent(sessionID), {
      method: 'DELETE',
      headers: getUploadHeaders(),
    });
  } catch (_) {
    // The server expires abandoned sessions even when best-effort cancellation fails.
  }
  activeUploadSession = null;
}

/**
 * @param {File} file
 * @param {HTMLFormElement} form
 * @returns {Promise<boolean>}
 */
async function resumableVideoUpload(file, form) {
  const result = document.getElementById('result');
  const preparationPath = chooseVideoPreparationPath(file);
  if (result instanceof HTMLElement) {
    result.textContent =
      preparationPath === 'direct'
        ? 'Preparing the direct H.264-compatible MP4 path…'
        : 'Preparing the server fallback path…';
    result.className = 'text-muted';
  }

  let fingerprint;
  try {
    fingerprint = await getFileFingerprint(file);
  } catch (_) {
    showUploadPreparationError(form, 'Could not read this file for resumable upload. Re-select it and try again.');
    return false;
  }

  const keepOriginalInput = form.querySelector('[name="keep_original"]');
  const keepOriginal = keepOriginalInput instanceof HTMLInputElement && keepOriginalInput.checked;
  const retentionInput = form.querySelector('[name="retention"]');
  const retentionDays = retentionInput instanceof HTMLSelectElement ? Number(retentionInput.value) : 7;
  let session = null;

  if (
    activeUploadSession &&
    activeUploadSession.serverSessionID &&
    activeUploadSession.resumeSupported &&
    activeUploadSession.fileFingerprint === fingerprint.value &&
    activeUploadSession.status === 'paused'
  ) {
    try {
      session = (await fetchUploadJSON('/upload/session/' + encodeURIComponent(activeUploadSession.serverSessionID)))?.session;
    } catch (_) {
      session = null;
    }
  }

  if (!session) {
    try {
      session = (
        await fetchUploadJSON('/upload/session', {
          method: 'POST',
          headers: { ...getUploadHeaders(), 'Content-Type': 'application/json' },
          body: JSON.stringify({
            filename: file.name,
            primary_filename: file.name,
            primary_size: file.size,
            primary_sha256: '',
            original_filename: keepOriginal ? file.name : '',
            original_size: keepOriginal ? file.size : 0,
            original_sha256: '',
            keep_original: keepOriginal,
            retention_days: retentionDays,
          }),
        })
      )?.session;
    } catch (error) {
      showUploadPreparationError(form, error instanceof Error ? error.message : 'Could not start upload.');
      return false;
    }
  }

  if (!session || !session.id || !Array.isArray(session.assets)) {
    showUploadPreparationError(form, 'The server returned an invalid upload session.');
    return false;
  }

  /** @type {UploadSession} */
  const clientSession = {
    uploadId: session.id,
    serverSessionID: session.id,
    fileFingerprint: fingerprint.value,
    resumeSupported: fingerprint.resumeSupported,
    totalChunks: 0,
    nextChunkIndex: 0,
    status: 'uploading',
  };
  activeUploadSession = clientSession;

  const totalBytes = session.assets.reduce((sum, asset) => sum + Number(asset.expected_size || 0), 0);
  let completedBytes = session.assets.reduce((sum, asset) => sum + Number(asset.received_bytes || 0), 0);

  for (const asset of session.assets) {
    const chunks = Array.isArray(asset.chunks) ? asset.chunks : [];
    const uploadedChunks = new Set(chunks.map((chunk) => Number(chunk.index)));
    const chunkSize = Number(asset.chunk_size || CHUNK_SIZE);
    const totalChunks = Number(asset.total_chunks || Math.ceil(file.size / chunkSize));
    for (let index = 0; index < totalChunks; index++) {
      if (uploadedChunks.has(index)) continue;
      const start = index * chunkSize;
      const end = Math.min(start + chunkSize, file.size);
      const chunk = file.slice(start, end);
      const chunkHash = await hashBlob(chunk);
      let uploaded = false;
      let permanentFailure = false;
      let lastError = null;
      for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
        try {
          await fetchUploadJSON(
            '/upload/session/' + encodeURIComponent(session.id) + '/assets/' + encodeURIComponent(asset.id) + '/chunks/' + index,
            {
              method: 'PUT',
              headers: { ...getUploadHeaders(), 'Content-Type': 'application/octet-stream', 'X-Chunk-SHA256': chunkHash },
              body: chunk,
            }
          );
          uploaded = true;
          break;
        } catch (error) {
          lastError = error;
          if (isPermanentUploadError(error)) {
            permanentFailure = true;
            break;
          }
          if (attempt < MAX_RETRIES) await new Promise((resolve) => setTimeout(resolve, 2 ** attempt * 1000));
        }
      }
      if (!uploaded) {
        if (permanentFailure) {
          await cancelServerUploadSession(session.id);
          if (result instanceof HTMLElement) {
            result.textContent = lastError instanceof Error ? lastError.message : 'Upload rejected.';
            result.className = 'text-error';
          }
          setUploadSubmitButtonLabel(form, 'Upload');
          return false;
        }
        clientSession.status = 'paused';
        if (result instanceof HTMLElement) {
          result.textContent = 'Upload paused. Retry to continue without re-sending completed chunks.';
          result.className = 'text-error';
        }
        setUploadSubmitButtonLabel(form, 'Resume upload');
        return false;
      }
      completedBytes += chunk.size;
      updateProgress((completedBytes / Math.max(totalBytes, 1)) * 90, 'Uploading ' + asset.role + '…');
    }

    updateProgress(Math.min((completedBytes / Math.max(totalBytes, 1)) * 90 + 5, 98), 'Finalizing ' + asset.role + '…');
    try {
      await fetchUploadJSON(
        '/upload/session/' + encodeURIComponent(session.id) + '/assets/' + encodeURIComponent(asset.id) + '/complete',
        { method: 'POST', headers: getUploadHeaders() }
      );
    } catch (error) {
      if (isPermanentUploadError(error)) {
        await cancelServerUploadSession(session.id);
        if (result instanceof HTMLElement) {
          result.textContent = error instanceof Error ? error.message : 'Upload finalization was rejected.';
          result.className = 'text-error';
        }
        setUploadSubmitButtonLabel(form, 'Upload');
        return false;
      }
      clientSession.status = 'paused';
      if (result instanceof HTMLElement) {
        result.textContent = error instanceof Error ? error.message : 'Upload finalization failed. Retry to continue.';
        result.className = 'text-error';
      }
      setUploadSubmitButtonLabel(form, 'Resume upload');
      return false;
    }
  }

  activeUploadSession = null;
  updateProgress(100, 'Done!');
  setUploadSubmitButtonLabel(form, 'Upload');
  window.location.href = '/';
  return true;
}
