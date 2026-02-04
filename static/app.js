// @ts-check

/**
 * Sharm client-side application
 * Type-checked via JSDoc + @ts-check
 */

// =============================================================================
// Constants
// =============================================================================

const CHUNK_SIZE = 5 * 1024 * 1024; // 5MB
const MAX_RETRIES = 3;

// =============================================================================
// Types
// =============================================================================

/**
 * @typedef {Object} ProbeResult
 * @property {number} duration - Duration in seconds
 * @property {number} width - Video width in pixels
 * @property {number} height - Video height in pixels
 * @property {number} size - File size in bytes
 */

// =============================================================================
// Utilities
// =============================================================================

/**
 * Generate a UUID v4
 * @returns {string}
 */
function generateUUID() {
  // Use crypto.randomUUID() when available (secure contexts)
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    try {
      return crypto.randomUUID();
    } catch (_) {
      // Fall through to fallback
    }
  }
  // Fallback for older browsers or non-secure contexts
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

/**
 * Format duration in seconds to HH:MM:SS or MM:SS
 * @param {number} sec - Duration in seconds
 * @returns {string}
 */
function formatDuration(sec) {
  if (!sec || !isFinite(sec)) return 'Unknown';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = Math.floor(sec % 60);
  if (h > 0)
    return h + ':' + String(m).padStart(2, '0') + ':' + String(s).padStart(2, '0');
  return m + ':' + String(s).padStart(2, '0');
}

/**
 * Format bytes to human readable size
 * @param {number} bytes
 * @returns {string}
 */
function formatSize(bytes) {
  if (bytes === 0) return '0 B';
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB';
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB';
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return bytes + ' B';
}

// =============================================================================
// Progress UI
// =============================================================================

/**
 * Update progress bar display
 * @param {number} percent - Progress percentage (0-100)
 * @param {string} [text] - Optional text to display instead of percentage
 */
function updateProgress(percent, text) {
  const bar = document.getElementById('upload-progress');
  const fill = document.getElementById('upload-progress-fill');
  const pct = document.getElementById('upload-progress-pct');
  if (!bar || !fill || !pct) return;
  bar.style.display = 'block';
  fill.style.width = percent + '%';
  pct.textContent = text || Math.round(percent) + '%';
}

/**
 * Hide progress bar
 */
function hideProgress() {
  const bar = document.getElementById('upload-progress');
  if (bar) bar.style.display = 'none';
}

// =============================================================================
// Client-side Media Probe
// =============================================================================

/**
 * Probe video/audio file metadata using browser APIs
 * @param {File} file - File to probe
 * @param {HTMLElement} container - Container to render results
 */
function probeClientSide(file, container) {
  const url = URL.createObjectURL(file);
  const isVideo =
    file.type.startsWith('video/') ||
    /\.(mp4|webm|mov|avi|mkv|flv|wmv|m4v)$/i.test(file.name);

  if (isVideo) {
    const video = document.createElement('video');
    video.preload = 'metadata';
    video.onloadedmetadata = function () {
      URL.revokeObjectURL(url);
      renderProbeResult(container, {
        duration: video.duration,
        width: video.videoWidth,
        height: video.videoHeight,
        size: file.size,
      });
    };
    video.onerror = function () {
      URL.revokeObjectURL(url);
      container.innerHTML =
        '<div class="text-muted" style="font-size:var(--text-xs);">Unable to read video metadata</div>';
    };
    video.src = url;
  } else {
    const audio = document.createElement('audio');
    audio.preload = 'metadata';
    audio.onloadedmetadata = function () {
      URL.revokeObjectURL(url);
      renderProbeResult(container, {
        duration: audio.duration,
        width: 0,
        height: 0,
        size: file.size,
      });
    };
    audio.onerror = function () {
      URL.revokeObjectURL(url);
      container.innerHTML =
        '<div class="text-muted" style="font-size:var(--text-xs);">Unable to read audio metadata</div>';
    };
    audio.src = url;
  }
}

/**
 * Render probe result to container
 * @param {HTMLElement} container
 * @param {ProbeResult} result
 */
function renderProbeResult(container, result) {
  let html =
    '<div style="background:var(--bg-elevated);border:1px solid var(--border);border-radius:var(--radius-md);padding:var(--s-sm) var(--s-md);font-size:var(--text-xs);">' +
    '<div style="display:flex;gap:var(--s-lg);flex-wrap:wrap;">' +
    '<div><span class="text-muted">Duration:</span> ' +
    formatDuration(result.duration) +
    '</div>';

  if (result.width > 0 && result.height > 0) {
    html +=
      '<div><span class="text-muted">Resolution:</span> ' +
      result.width +
      '×' +
      result.height +
      '</div>';
  }

  html +=
    '<div><span class="text-muted">Size:</span> ' +
    formatSize(result.size) +
    '</div>' +
    '</div></div>';

  container.innerHTML = html;
}

// =============================================================================
// Chunked Upload
// =============================================================================

/**
 * Upload a single chunk with retry logic
 * @param {string} uploadId - Unique upload identifier
 * @param {number} chunkIndex - Index of this chunk
 * @param {Blob} chunk - Chunk data
 * @param {number} maxRetries - Maximum retry attempts
 * @returns {Promise<boolean>} - True if successful
 */
async function uploadChunk(uploadId, chunkIndex, chunk, maxRetries) {
  const fd = new FormData();
  fd.append('uploadId', uploadId);
  fd.append('chunkIndex', String(chunkIndex));
  fd.append('chunk', chunk);

  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      const resp = await fetch('/upload/chunk', { method: 'POST', body: fd });
      if (resp.ok) return true;
      // Don't retry on client errors (4xx) - these won't succeed on retry
      if (resp.status < 500) return false;
      // Retry on server errors (5xx)
      if (attempt < maxRetries) {
        await new Promise((r) => setTimeout(r, Math.pow(2, attempt) * 1000));
      }
    } catch (_) {
      // Network error - retry with backoff
      if (attempt < maxRetries) {
        await new Promise((r) => setTimeout(r, Math.pow(2, attempt) * 1000));
      }
    }
  }
  return false;
}

/**
 * Perform chunked upload of a file
 * @param {File} file - File to upload
 * @param {HTMLFormElement} form - Form element with settings
 * @returns {Promise<boolean>} - True if successful
 */
async function chunkedUpload(file, form) {
  const uploadId = generateUUID();
  const totalChunks = Math.ceil(file.size / CHUNK_SIZE);
  const result = document.getElementById('result');

  for (let i = 0; i < totalChunks; i++) {
    const start = i * CHUNK_SIZE;
    const end = Math.min(start + CHUNK_SIZE, file.size);
    const chunk = file.slice(start, end);

    updateProgress(
      (i / totalChunks) * 90,
      'Uploading chunk ' + (i + 1) + '/' + totalChunks
    );

    const ok = await uploadChunk(uploadId, i, chunk, MAX_RETRIES);
    if (!ok) {
      if (result) {
        result.innerHTML =
          '<div class="text-error" style="font-size:var(--text-sm);">Upload failed at chunk ' +
          (i + 1) +
          '. Please try again.</div>';
      }
      hideProgress();
      return false;
    }
  }

  updateProgress(95, 'Finalizing...');

  const fd = new FormData();
  fd.append('uploadId', uploadId);
  fd.append('filename', file.name);
  fd.append('totalChunks', String(totalChunks));

  const retentionSelect = form.querySelector('[name="retention"]');
  if (retentionSelect instanceof HTMLSelectElement) {
    fd.append('retention', retentionSelect.value);
  }

  form.querySelectorAll('[name="codecs"]:checked').forEach((cb) => {
    if (cb instanceof HTMLInputElement) {
      fd.append('codecs', cb.value);
    }
  });

  const fpsInput = form.querySelector('[name="fps"]:checked');
  if (fpsInput instanceof HTMLInputElement) {
    fd.append('fps', fpsInput.value);
  }

  try {
    const resp = await fetch('/upload/complete', { method: 'POST', body: fd });
    if (resp.ok) {
      const redirect = resp.headers.get('HX-Redirect');
      if (redirect) {
        window.location.href = redirect;
      } else {
        updateProgress(100, 'Done!');
        window.location.href = '/';
      }
      return true;
    } else {
      const text = await resp.text();
      if (result) {
        result.innerHTML =
          text ||
          '<div class="text-error" style="font-size:var(--text-sm);">Upload failed</div>';
      }
      hideProgress();
      return false;
    }
  } catch (e) {
    if (result) {
      result.innerHTML =
        '<div class="text-error" style="font-size:var(--text-sm);">Upload failed. Please try again.</div>';
    }
    hideProgress();
    return false;
  }
}

// =============================================================================
// Upload Page Initialization
// =============================================================================

/**
 * Initialize upload page functionality
 */
function initUploadPage() {
  const form = document.getElementById('upload-form');
  if (!(form instanceof HTMLFormElement)) return;

  const fileInput = form.querySelector('input[name="file"]');
  if (!(fileInput instanceof HTMLInputElement)) return;

  // File selection handler - use addEventListener instead of mutating onchange attribute
  if (!fileInput.dataset.listenerAttached) {
    fileInput.addEventListener('change', function () {
      window.handleFileSelect(this);
    });
    fileInput.dataset.listenerAttached = 'true';
  }

  // Form submit handler
  form.addEventListener('submit', async function (e) {
    e.preventDefault();
    const file = fileInput.files?.[0];
    if (!file) {
      const result = document.getElementById('result');
      if (result) {
        result.innerHTML =
          '<div class="text-error" style="font-size:var(--text-sm);">Please select a file</div>';
      }
      return;
    }
    const submitBtn = form.querySelector('button[type="submit"]');
    if (submitBtn instanceof HTMLButtonElement) {
      submitBtn.disabled = true;
    }
    const result = document.getElementById('result');
    if (result) result.innerHTML = '';

    await chunkedUpload(file, form);

    if (submitBtn instanceof HTMLButtonElement) {
      submitBtn.disabled = false;
    }
  });

  // Codec checkbox change handler for FPS visibility
  document.querySelectorAll('#codec-av1 input, #codec-h264 input').forEach((cb) => {
    cb.addEventListener('change', updateFpsVisibility);
  });
}

/**
 * Handle file selection - update codec options and probe
 * @param {HTMLInputElement} input
 */
function handleFileSelect(input) {
  const opts = document.getElementById('codec-options');
  const av1 = document.getElementById('codec-av1');
  const h264 = document.getElementById('codec-h264');
  const opus = document.getElementById('codec-opus');
  const fpsOpts = document.getElementById('fps-options');
  const probeResult = document.getElementById('probe-result');

  if (!input.files?.[0]) {
    if (opts) opts.style.display = 'none';
    if (fpsOpts) fpsOpts.style.display = 'none';
    return;
  }

  const name = input.files[0].name.toLowerCase();
  const videoExts = ['.mp4', '.webm', '.mov', '.avi', '.mkv', '.flv', '.wmv', '.m4v'];
  const audioExts = ['.mp3', '.wav', '.ogg', '.flac', '.aac', '.m4a', '.wma', '.opus'];
  const isVideo = videoExts.some((e) => name.endsWith(e));
  const isAudio = audioExts.some((e) => name.endsWith(e));

  if (isVideo) {
    if (opts) opts.style.display = 'block';
    if (av1) av1.style.display = 'flex';
    if (h264) h264.style.display = 'flex';
    if (opus) opus.style.display = 'none';
    updateFpsVisibility();
  } else if (isAudio) {
    if (opts) opts.style.display = 'block';
    if (av1) av1.style.display = 'none';
    if (h264) h264.style.display = 'none';
    if (opus) opus.style.display = 'flex';
    if (fpsOpts) fpsOpts.style.display = 'none';
  } else {
    if (opts) opts.style.display = 'none';
    if (fpsOpts) fpsOpts.style.display = 'none';
  }

  if (probeResult && (isVideo || isAudio)) {
    probeClientSide(input.files[0], probeResult);
  } else if (probeResult) {
    probeResult.innerHTML = '';
  }
}

/**
 * Update FPS options visibility based on codec selection
 */
function updateFpsVisibility() {
  const fpsOpts = document.getElementById('fps-options');
  const av1Input = document.querySelector('#codec-av1 input');
  const h264Input = document.querySelector('#codec-h264 input');

  const av1Checked = av1Input instanceof HTMLInputElement && av1Input.checked;
  const h264Checked = h264Input instanceof HTMLInputElement && h264Input.checked;

  if (fpsOpts) {
    fpsOpts.style.display = av1Checked || h264Checked ? 'block' : 'none';
  }
}

// =============================================================================
// Global Exports (for inline handlers)
// =============================================================================

// @ts-ignore
window.handleFileSelect = handleFileSelect;

// =============================================================================
// Dashboard Page
// =============================================================================

/**
 * Initialize dashboard page functionality
 */
function initDashboardPage() {
  // Handle empty media list after delete
  document.body.addEventListener('htmx:afterRequest', function (e) {
    // Guard: check that detail and requestConfig exist
    if (!e.detail || !e.detail.requestConfig) return;
    if (e.detail.requestConfig.verb !== 'delete') return;

    var list = document.getElementById('media-list');
    if (list && list.children.length === 0) {
      list.outerHTML =
        '<div class="card"><div style="text-align:center;padding:var(--s-2xl) var(--s-md);">' +
        '<p style="color:var(--text-muted);font-size:var(--text-sm);">No media yet. Upload something to get started.</p></div></div>';
    }
  });

  // Handle info dialog opening after swap
  document.body.addEventListener('htmx:afterSwap', function (e) {
    // FIX: Guard against missing detail.target (SSE events)
    if (!e.detail || !e.detail.target) return;
    if (e.detail.target.id === 'info-dialog-content') {
      var dialog = document.getElementById('info-dialog');
      if (dialog) dialog.showModal();
    }
  });
}

// =============================================================================
// Dialog Utilities
// =============================================================================

/**
 * Close dialog when clicking on backdrop
 * @param {Event} event - Click event
 * @param {HTMLDialogElement} dialog - Dialog element
 */
function closeDialogOnBackdrop(event, dialog) {
  if (event.target === dialog) {
    dialog.close();
  }
}

// =============================================================================
// Confirm Dialog (HTMX)
// =============================================================================

/**
 * Initialize confirm dialog for HTMX delete confirmations
 */
function initConfirmDialog() {
  var dialog = document.getElementById('confirm-dialog');
  var msg = document.getElementById('confirm-dialog-msg');
  if (!dialog || !msg) return;

  var pendingEvt = null;

  document.body.addEventListener('htmx:confirm', function (e) {
    if (!e.detail || !e.detail.question) return;
    e.preventDefault();
    msg.textContent = e.detail.question;
    pendingEvt = e;
    dialog.showModal();
  });

  dialog.addEventListener('close', function () {
    if (dialog.returnValue === 'confirm' && pendingEvt) {
      pendingEvt.detail.issueRequest(true);
    }
    pendingEvt = null;
  });

  dialog.addEventListener('click', function (e) {
    if (e.target === dialog) dialog.close('cancel');
  });
}

// =============================================================================
// Global Exports (for inline handlers)
// =============================================================================

// @ts-ignore
window.closeDialogOnBackdrop = closeDialogOnBackdrop;

// =============================================================================
// Auto-initialization
// =============================================================================

document.addEventListener('DOMContentLoaded', function () {
  initUploadPage();
  initDashboardPage();
  initConfirmDialog();
});
