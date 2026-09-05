(function () {
  'use strict';

  var ROOT_ID = 'readr-bookmarklet-root';

  // 1. Prevent duplicate modals
  var existingRoot = document.getElementById(ROOT_ID);
  if (existingRoot && existingRoot.shadowRoot) {
    var existingInput = existingRoot.shadowRoot.getElementById('readr-tags-input');
    if (existingInput) {
      existingInput.focus();
    }
    return;
  }

  // 2. Resolve metadata from host page
  function getPageTitle() {
    var ogTitle = document.querySelector('meta[property="og:title"]');
    if (ogTitle && ogTitle.getAttribute('content')) {
      var content = ogTitle.getAttribute('content').trim();
      if (content) return content;
    }
    var twitterTitle = document.querySelector('meta[name="twitter:title"]');
    if (twitterTitle && twitterTitle.getAttribute('content')) {
      var tContent = twitterTitle.getAttribute('content').trim();
      if (tContent) return tContent;
    }
    return (document.title || '').trim() || 'Untitled Article';
  }

  function getPageUrl() {
    var canonical = document.querySelector('link[rel="canonical"]');
    if (canonical && canonical.getAttribute('href')) {
      var href = canonical.getAttribute('href').trim();
      if (href) {
        try {
          return new URL(href, window.location.href).href;
        } catch (e) {
          // fallback
        }
      }
    }
    var ogUrl = document.querySelector('meta[property="og:url"]');
    if (ogUrl && ogUrl.getAttribute('content')) {
      var ogHref = ogUrl.getAttribute('content').trim();
      if (ogHref) {
        try {
          return new URL(ogHref, window.location.href).href;
        } catch (e) {}
      }
    }
    return window.location.href;
  }

  // 3. Resolve Readr Server URL & Auth Token
  function getServerUrl() {
    if (window.__READR_SERVER_URL__) {
      return String(window.__READR_SERVER_URL__).replace(/\/+$/, '');
    }
    var script = document.currentScript;
    if (script && script.dataset && script.dataset.serverUrl) {
      return String(script.dataset.serverUrl).replace(/\/+$/, '');
    }
    if (script && script.src) {
      try {
        var parsed = new URL(script.src);
        if (parsed.origin && parsed.origin !== 'null') {
          return parsed.origin;
        }
      } catch (e) {}
    }
    return 'http://localhost:8080';
  }

  function getApiKey() {
    if (window.__READR_API_KEY__) {
      return String(window.__READR_API_KEY__).trim();
    }
    if (window.__READR_AUTH_TOKEN__) {
      return String(window.__READR_AUTH_TOKEN__).trim();
    }
    var script = document.currentScript;
    if (script && script.dataset && script.dataset.apiKey) {
      return String(script.dataset.apiKey).trim();
    }
    if (script && script.dataset && script.dataset.authToken) {
      return String(script.dataset.authToken).trim();
    }
    return '';
  }

  var initialTitle = getPageTitle();
  var initialUrl = getPageUrl();
  var serverUrl = getServerUrl();
  var apiKey = getApiKey();

  // 4. Create host element and attach open Shadow DOM
  var host = document.createElement('div');
  host.id = ROOT_ID;
  var shadow = host.attachShadow({ mode: 'open' });

  // 5. CSS Styles
  var style = document.createElement('style');
  style.textContent = `
    :host {
      --readr-font: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Geist", "Outfit", Helvetica, Arial, sans-serif;
      --readr-font-mono: "Geist Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      
      --readr-bg: #11141a;
      --readr-card-bg: rgba(23, 27, 34, 0.95);
      --readr-border: rgba(255, 255, 255, 0.12);
      --readr-border-focus: #3b82f6;
      --readr-text: #f3f4f6;
      --readr-text-muted: #9ca3af;
      --readr-input-bg: rgba(15, 18, 24, 0.8);
      --readr-primary: #3b82f6;
      --readr-primary-hover: #2563eb;
      --readr-primary-text: #ffffff;
      --readr-btn-secondary-bg: rgba(255, 255, 255, 0.08);
      --readr-btn-secondary-hover: rgba(255, 255, 255, 0.14);
      --readr-btn-secondary-text: #e5e7eb;
      --readr-success-bg: rgba(16, 185, 129, 0.15);
      --readr-success-border: rgba(16, 185, 129, 0.35);
      --readr-success-text: #34d399;
      --readr-error-bg: rgba(239, 68, 68, 0.15);
      --readr-error-border: rgba(239, 68, 68, 0.35);
      --readr-error-text: #f87171;
      --readr-warn-bg: rgba(245, 158, 11, 0.15);
      --readr-warn-border: rgba(245, 158, 11, 0.35);
      --readr-warn-text: #fbbf24;
      --readr-shadow: 0 20px 40px -10px rgba(0, 0, 0, 0.6), 0 0 0 1px rgba(255, 255, 255, 0.1);
      
      all: initial;
      z-index: 2147483647;
      position: fixed;
      inset: 0;
      pointer-events: auto;
      font-family: var(--readr-font);
    }

    @media (prefers-color-scheme: light) {
      :host {
        --readr-bg: #f9fafb;
        --readr-card-bg: rgba(255, 255, 255, 0.98);
        --readr-border: rgba(0, 0, 0, 0.1);
        --readr-border-focus: #2563eb;
        --readr-text: #111827;
        --readr-text-muted: #6b7280;
        --readr-input-bg: #f3f4f6;
        --readr-primary: #2563eb;
        --readr-primary-hover: #1d4ed8;
        --readr-primary-text: #ffffff;
        --readr-btn-secondary-bg: #e5e7eb;
        --readr-btn-secondary-hover: #d1d5db;
        --readr-btn-secondary-text: #374151;
        --readr-success-bg: #ecfdf5;
        --readr-success-border: #a7f3d0;
        --readr-success-text: #059669;
        --readr-error-bg: #fef2f2;
        --readr-error-border: #fecaca;
        --readr-error-text: #dc2626;
        --readr-warn-bg: #fffbeb;
        --readr-warn-border: #fde68a;
        --readr-warn-text: #d97706;
        --readr-shadow: 0 20px 40px -10px rgba(0, 0, 0, 0.18), 0 0 0 1px rgba(0, 0, 0, 0.08);
      }
    }

    .readr-backdrop {
      position: fixed;
      inset: 0;
      background-color: rgba(0, 0, 0, 0.45);
      backdrop-filter: blur(4px);
      -webkit-backdrop-filter: blur(4px);
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 16px;
      animation: readrFadeIn 0.15s ease-out forwards;
      box-sizing: border-box;
    }

    .readr-dialog {
      background: var(--readr-card-bg);
      color: var(--readr-text);
      width: 100%;
      max-width: 440px;
      border-radius: 16px;
      border: 1px solid var(--readr-border);
      box-shadow: var(--readr-shadow);
      padding: 24px;
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      gap: 16px;
      animation: readrScaleIn 0.2s cubic-bezier(0.16, 1, 0.3, 1) forwards;
    }

    @keyframes readrFadeIn {
      from { opacity: 0; }
      to { opacity: 1; }
    }

    @keyframes readrScaleIn {
      from {
        opacity: 0;
        transform: scale(0.95) translateY(-8px);
      }
      to {
        opacity: 1;
        transform: scale(1) translateY(0);
      }
    }

    .readr-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin: 0;
      padding: 0;
    }

    .readr-title-group {
      display: flex;
      align-items: center;
      gap: 10px;
    }

    .readr-logo-icon {
      font-size: 20px;
      line-height: 1;
    }

    .readr-heading {
      font-size: 17px;
      font-weight: 600;
      letter-spacing: -0.01em;
      margin: 0;
      color: var(--readr-text);
    }

    .readr-close-btn {
      background: transparent;
      border: none;
      color: var(--readr-text-muted);
      cursor: pointer;
      font-size: 18px;
      padding: 4px 8px;
      border-radius: 6px;
      line-height: 1;
      display: flex;
      align-items: center;
      justify-content: center;
      transition: background 0.12s ease, color 0.12s ease;
    }

    .readr-close-btn:hover {
      background: var(--readr-btn-secondary-bg);
      color: var(--readr-text);
    }

    .readr-preview-box {
      background: var(--readr-input-bg);
      border: 1px solid var(--readr-border);
      border-radius: 10px;
      padding: 12px;
      display: flex;
      flex-direction: column;
      gap: 4px;
    }

    .readr-doc-title {
      font-size: 13.5px;
      font-weight: 500;
      line-height: 1.35;
      color: var(--readr-text);
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
      overflow: hidden;
      word-break: break-word;
    }

    .readr-doc-url {
      font-size: 11.5px;
      font-family: var(--readr-font-mono);
      color: var(--readr-text-muted);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      text-decoration: none;
      display: block;
    }

    .readr-form-group {
      display: flex;
      flex-direction: column;
      gap: 6px;
    }

    .readr-label {
      font-size: 12px;
      font-weight: 500;
      color: var(--readr-text-muted);
      text-transform: uppercase;
      letter-spacing: 0.04em;
    }

    .readr-input {
      background: var(--readr-input-bg);
      border: 1px solid var(--readr-border);
      border-radius: 8px;
      padding: 9px 12px;
      font-size: 13.5px;
      color: var(--readr-text);
      outline: none;
      box-sizing: border-box;
      width: 100%;
      font-family: inherit;
      transition: border-color 0.15s ease, box-shadow 0.15s ease;
    }

    .readr-input:focus {
      border-color: var(--readr-border-focus);
      box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.25);
    }

    .readr-input::placeholder {
      color: var(--readr-text-muted);
      opacity: 0.7;
    }

    .readr-banner {
      display: none;
      padding: 10px 12px;
      border-radius: 8px;
      font-size: 13px;
      line-height: 1.4;
      align-items: center;
      gap: 8px;
      word-break: break-word;
      animation: readrFadeIn 0.15s ease-out;
    }

    .readr-banner.visible {
      display: flex;
    }

    .readr-banner.success {
      background: var(--readr-success-bg);
      border: 1px solid var(--readr-success-border);
      color: var(--readr-success-text);
    }

    .readr-banner.warn {
      background: var(--readr-warn-bg);
      border: 1px solid var(--readr-warn-border);
      color: var(--readr-warn-text);
    }

    .readr-banner.error {
      background: var(--readr-error-bg);
      border: 1px solid var(--readr-error-border);
      color: var(--readr-error-text);
    }

    .readr-banner a {
      color: inherit;
      font-weight: 600;
      text-decoration: underline;
      cursor: pointer;
    }

    .readr-actions {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      gap: 10px;
      margin-top: 4px;
    }

    .readr-btn {
      font-family: inherit;
      font-size: 13.5px;
      font-weight: 500;
      border-radius: 8px;
      padding: 8px 16px;
      cursor: pointer;
      border: none;
      outline: none;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      transition: background 0.12s ease, opacity 0.12s ease, transform 0.08s ease;
    }

    .readr-btn:active {
      transform: scale(0.98);
    }

    .readr-btn-secondary {
      background: var(--readr-btn-secondary-bg);
      color: var(--readr-btn-secondary-text);
    }

    .readr-btn-secondary:hover {
      background: var(--readr-btn-secondary-hover);
    }

    .readr-btn-primary {
      background: var(--readr-primary);
      color: var(--readr-primary-text);
    }

    .readr-btn-primary:hover {
      background: var(--readr-primary-hover);
    }

    .readr-btn:disabled {
      opacity: 0.55;
      cursor: not-allowed;
      pointer-events: none;
    }

    .readr-spinner {
      width: 14px;
      height: 14px;
      border: 2px solid rgba(255, 255, 255, 0.3);
      border-radius: 50%;
      border-top-color: #fff;
      animation: readrSpin 0.7s linear infinite;
      display: inline-block;
    }

    @keyframes readrSpin {
      to { transform: rotate(360deg); }
    }
  `;

  // 6. Template HTML
  var wrapper = document.createElement('div');
  wrapper.className = 'readr-backdrop';
  wrapper.innerHTML = `
    <div class="readr-dialog" role="dialog" aria-modal="true" aria-labelledby="readr-dialog-title">
      <div class="readr-header">
        <div class="readr-title-group">
          <span class="readr-logo-icon" aria-hidden="true">📚</span>
          <h2 id="readr-dialog-title" class="readr-heading">Save to Readr</h2>
        </div>
        <button type="button" class="readr-close-btn" id="readr-close-btn" aria-label="Close dialog">&times;</button>
      </div>

      <div class="readr-preview-box">
        <div class="readr-doc-title" id="readr-preview-title"></div>
        <div class="readr-doc-url" id="readr-preview-url"></div>
      </div>

      <div class="readr-form-group">
        <label for="readr-tags-input" class="readr-label">Tags (comma-separated)</label>
        <input type="text" id="readr-tags-input" class="readr-input" placeholder="tech, research, ai..." autocomplete="off" spellcheck="false" />
      </div>

      <div id="readr-status-banner" class="readr-banner" role="status"></div>

      <div class="readr-actions">
        <button type="button" id="readr-cancel-btn" class="readr-btn readr-btn-secondary">Cancel</button>
        <button type="button" id="readr-save-btn" class="readr-btn readr-btn-primary">
          <span id="readr-save-text">Save to Vault</span>
        </button>
      </div>
    </div>
  `;

  shadow.appendChild(style);
  shadow.appendChild(wrapper);

  // 7. Setup element references
  var previewTitle = shadow.getElementById('readr-preview-title');
  var previewUrl = shadow.getElementById('readr-preview-url');
  var tagsInput = shadow.getElementById('readr-tags-input');
  var statusBanner = shadow.getElementById('readr-status-banner');
  var saveBtn = shadow.getElementById('readr-save-btn');
  var saveText = shadow.getElementById('readr-save-text');
  var cancelBtn = shadow.getElementById('readr-cancel-btn');
  var closeBtn = shadow.getElementById('readr-close-btn');

  previewTitle.textContent = initialTitle;
  previewUrl.textContent = initialUrl;

  var isSubmitting = false;
  var dismissTimer = null;

  function cleanup() {
    if (dismissTimer) clearTimeout(dismissTimer);
    document.removeEventListener('keydown', handleGlobalKeydown, true);
    if (host.parentNode) {
      host.parentNode.removeChild(host);
    }
  }

  function showStatus(type, messageHtml, autoDismissDelay) {
    statusBanner.className = 'readr-banner visible ' + type;
    statusBanner.innerHTML = messageHtml;
    if (autoDismissDelay) {
      if (dismissTimer) clearTimeout(dismissTimer);
      dismissTimer = setTimeout(function () {
        cleanup();
      }, autoDismissDelay);
    }
  }

  function setPending(pending) {
    isSubmitting = pending;
    saveBtn.disabled = pending;
    cancelBtn.disabled = pending;
    tagsInput.disabled = pending;
    if (pending) {
      saveText.innerHTML = '<span class="readr-spinner" aria-hidden="true"></span> Saving...';
    } else {
      saveText.textContent = 'Save to Vault';
    }
  }

  function escapeHtml(str) {
    if (!str) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function parseTags(raw) {
    if (!raw) return [];
    return raw
      .split(',')
      .map(function (t) { return t.trim(); })
      .filter(function (t) { return t.length > 0; });
  }

  function doSave() {
    if (isSubmitting) return;
    var rawTags = tagsInput.value.trim();
    var tagsArray = rawTags
      ? rawTags
          .split(',')
          .map(function (t) {
            return t.trim();
          })
          .filter(Boolean)
      : [];

    var payload = {
      url: initialUrl,
      tags: tagsArray
    };

    var headers = {
      'Content-Type': 'application/json'
    };
    if (apiKey) {
      headers['Authorization'] = 'Bearer ' + apiKey;
      headers['X-API-Key'] = apiKey;
    }

    setPending(true);
    showStatus('info', 'Saving article to vault...');

    fetch(serverUrl + '/api/add', {
      method: 'POST',
      headers: headers,
      body: JSON.stringify(payload)
    })
      .then(function (res) {
        if (res.status === 401 || res.status === 403) {
          throw { isAuth: true, message: 'Authentication required. Please check your API key in Readr Settings.' };
        }
        return res.json().then(
          function (data) {
            return { ok: res.ok, status: res.status, data: data };
          },
          function () {
            if (!res.ok) {
              throw { message: 'Server responded with status ' + res.status };
            }
            return { ok: res.ok, status: res.status, data: {} };
          }
        );
      })
      .then(function (result) {
        setPending(false);
        var data = result.data || {};
        var articleId = data.id || data.ID;
        var articleLink = articleId ? (serverUrl + '/#/article/' + articleId) : (serverUrl + '/');

        if (data.status === 'exists') {
          showStatus('warn', 'Article already in vault. <a href="' + articleLink + '" target="_blank" rel="noopener noreferrer">Open in Readr &rarr;</a>', 0);
          return;
        }

        if (result.ok || data.status === 'success') {
          showStatus('success', 'Saved to Vault! <a href="' + articleLink + '" target="_blank" rel="noopener noreferrer">Open in Readr &rarr;</a>', 1500);
          return;
        }

        showStatus('error', escapeHtml(data.message) || 'Failed to save article.');
      })
      .catch(function (err) {
        setPending(false);
        if (err && err.isAuth) {
          showStatus('error', escapeHtml(err.message));
        } else if (err && err.name === 'TypeError' && String(err.message).indexOf('fetch') !== -1) {
          showStatus('error', 'Unable to reach Readr server at ' + escapeHtml(serverUrl) + '. Ensure server is running.');
        } else {
          showStatus('error', escapeHtml((err && err.message) || 'An unexpected error occurred while saving.'));
        }
      });
  }

  // 8. Event handlers
  function handleGlobalKeydown(e) {
    if (e.key === 'Escape') {
      e.stopPropagation();
      e.preventDefault();
      cleanup();
    } else if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.stopPropagation();
      e.preventDefault();
      doSave();
    }
  }

  tagsInput.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
      e.preventDefault();
      e.stopPropagation();
      doSave();
    }
  });

  wrapper.addEventListener('click', function (e) {
    if (e.target === wrapper) {
      cleanup();
    }
  });

  closeBtn.addEventListener('click', cleanup);
  cancelBtn.addEventListener('click', cleanup);
  saveBtn.addEventListener('click', doSave);

  document.addEventListener('keydown', handleGlobalKeydown, true);

  // 9. Mount and focus
  document.body.appendChild(host);
  setTimeout(function () {
    if (tagsInput) tagsInput.focus();
  }, 50);
})();
