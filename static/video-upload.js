/**
 * @typedef {Object} PreparedVideo
 * @property {Blob} blob
 * @property {string} filename
 * @property {'direct' | 'client-encoding' | 'server-fallback'} path
 * @property {string} warning
 */

/**
 * @param {File} file
 * @returns {boolean}
 */
function isVideoFile(file) {
  return file.type.startsWith('video/') || /\.(mp4|webm|mov|avi|mkv|flv|wmv|m4v)$/i.test(file.name);
}

const CLIENT_VIDEO_WORKER_URL = '/static/client-video-worker.js';
const CLIENT_VIDEO_WORKER_IDLE_TIMEOUT_MS = 5 * 60 * 1000;

/**
 * The Worker inspects the actual tracks before choosing the direct path. Any
 * unsupported input or codec capability falls back to the server. The server
 * remains authoritative and validates every produced MP4 with ffprobe.
 * @param {File} file
 * @param {(progress: number) => void} onProgress
 * @param {number} maxInputBytes
 * @returns {Promise<{blob: Blob, filename: string, path: 'direct' | 'client-encoding' | 'server-fallback', warning: string}>}
 */
async function prepareVideoForUpload(file, onProgress, maxInputBytes) {
  if (!('Worker' in globalThis)) {
    return { blob: file, filename: file.name, path: 'server-fallback', warning: 'Web Workers are unavailable.' };
  }

  return new Promise((resolve) => {
    let settled = false;
    let worker;
    try {
      worker = new Worker(CLIENT_VIDEO_WORKER_URL);
    } catch (_) {
      resolve({ blob: file, filename: file.name, path: 'server-fallback', warning: 'Client encoding could not start.' });
      return;
    }
    let watchdogID = 0;
    /** @param {PreparedVideo} prepared */
    const finish = (prepared) => {
      if (settled) return;
      settled = true;
      clearTimeout(watchdogID);
      worker.terminate();
      resolve(prepared);
    };
    const resetWatchdog = () => {
      clearTimeout(watchdogID);
      watchdogID = setTimeout(() => {
        finish({ blob: file, filename: file.name, path: 'server-fallback', warning: 'Client encoding stopped responding.' });
      }, CLIENT_VIDEO_WORKER_IDLE_TIMEOUT_MS);
    };
    worker.addEventListener('error', () => {
      finish({ blob: file, filename: file.name, path: 'server-fallback', warning: 'Client encoding could not start.' });
    });
    worker.addEventListener('message', (event) => {
      resetWatchdog();
      const message = event.data;
      if (message?.type === 'progress') {
        onProgress(Number(message.progress) || 0);
        return;
      }
      if (message?.type === 'direct') {
        finish({ blob: file, filename: file.name, path: 'direct', warning: '' });
        return;
      }
      if (message?.type === 'encoded' && message.blob instanceof Blob) {
        if (Number.isFinite(maxInputBytes) && message.blob.size > maxInputBytes) {
          finish({
            blob: file,
            filename: file.name,
            path: 'server-fallback',
            warning: 'The client-produced MP4 exceeds the configured upload size.',
          });
          return;
        }
        const basename = file.name.replace(/\.[^.]+$/, '') || 'video';
        finish({
          blob: message.blob,
          filename: basename + '.mp4',
          path: 'client-encoding',
          warning: '',
        });
        return;
      }
      if (message?.type === 'fallback') {
        finish({
          blob: file,
          filename: file.name,
          path: 'server-fallback',
          warning: typeof message.error === 'string' ? message.error : 'Client encoding is unavailable.',
        });
      }
    });
    resetWatchdog();
    try {
      worker.postMessage({ type: 'prepare', file, maxInputBytes });
    } catch (_) {
      finish({ blob: file, filename: file.name, path: 'server-fallback', warning: 'The video could not be sent to the client encoder.' });
    }
  });
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
  let fingerprint;
  try {
    fingerprint = await getFileFingerprint(file);
  } catch (_) {
    showUploadPreparationError(form, 'Could not read this file for resumable upload. Re-select it and try again.');
    return false;
  }

  const configuredMaxSizeMB = Number(form.dataset.maxUploadSizeMb);
  const maxInputBytes = Number.isFinite(configuredMaxSizeMB) && configuredMaxSizeMB > 0
    ? configuredMaxSizeMB * 1024 * 1024
    : Number.POSITIVE_INFINITY;
  if (file.size > maxInputBytes) {
    showUploadPreparationError(form, 'This video exceeds the configured upload size.');
    return false;
  }

  /** @type {PreparedVideo} */
  let prepared;
  if (
    activeUploadSession?.status === 'paused' &&
    activeUploadSession.fileFingerprint === fingerprint.value &&
    activeUploadSession.preparedPrimary &&
    activeUploadSession.preparationPath
  ) {
    prepared = {
      blob: activeUploadSession.preparedPrimary,
      filename: activeUploadSession.preparationPath === 'client-encoding'
        ? (file.name.replace(/\.[^.]+$/, '') || 'video') + '.mp4'
        : file.name,
      path: activeUploadSession.preparationPath,
      warning: '',
    };
  } else {
    if (result instanceof HTMLElement) {
      result.textContent = 'Inspecting video and browser codec support…';
      result.className = 'text-muted';
    }
    prepared = await prepareVideoForUpload(file, (progress) => {
      updateProgress(Math.min(Math.max(progress, 0), 1) * 35, 'Encoding H.264 on this device…');
    }, maxInputBytes);
  }

  if (result instanceof HTMLElement) {
    if (prepared.path === 'direct') {
      result.textContent = 'The video is already H.264/AAC and will be uploaded directly.';
    } else if (prepared.path === 'client-encoding') {
      result.textContent = 'Client-side H.264 encoding complete. Uploading the optimized MP4…';
    } else {
      result.textContent = 'Client encoding unavailable; using the server fallback. ' + prepared.warning;
    }
    result.className = 'text-muted';
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
            primary_filename: prepared.filename,
            primary_size: prepared.blob.size,
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
    preparedPrimary: prepared.blob,
    preparationPath: prepared.path,
    status: 'uploading',
  };
  activeUploadSession = clientSession;

  const sessionAssets = /** @type {Array<any>} */ (session.assets);
  const totalBytes = sessionAssets.reduce((sum, asset) => sum + Number(asset.expected_size || 0), 0);
  let completedBytes = sessionAssets.reduce((sum, asset) => sum + Number(asset.received_bytes || 0), 0);

  for (const asset of sessionAssets) {
    const assetBlob = asset.role === 'original' ? file : prepared.blob;
    const chunks = /** @type {Array<any>} */ (Array.isArray(asset.chunks) ? asset.chunks : []);
    const uploadedChunks = new Set(chunks.map((chunk) => Number(chunk.index)));
    const chunkSize = Number(asset.chunk_size || CHUNK_SIZE);
    const totalChunks = Number(asset.total_chunks || Math.ceil(assetBlob.size / chunkSize));
    for (let index = 0; index < totalChunks; index++) {
      if (uploadedChunks.has(index)) continue;
      const start = index * chunkSize;
      const end = Math.min(start + chunkSize, assetBlob.size);
      const chunk = assetBlob.slice(start, end);
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
