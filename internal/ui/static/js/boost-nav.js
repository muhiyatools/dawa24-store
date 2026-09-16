// boost-nav.js -- in-place navigation for the dashboard shells.
//
// hx-boost sits on .app-shell. Every boosted response is reduced to the inner
// HTML of <main id="main-content"> and swapped into the current <main>. The
// sidebar is never replaced. Anything that cannot be swapped safely -- another
// shell, a download, a page that opted out with data-hard-nav -- falls back to
// an ordinary page load, so the worst case is the behaviour before boosting.
(function () {
  'use strict';
  if (!window.htmx) return;

  var MAIN_ID = 'main-content';
  var HARD_NAV_MARK = 'data-hard-nav';
  var DOWNLOAD_PATH = /\/(export|download|print)(\/|$)|\.(?:csv|xlsx?|pdf|zip|png|jpe?g|webp)$/i;
  var FULL_LOAD_PATH = /^\/(auth|api|lang)\/|\/set-branch$|\/logout$/;
  var LINK_OPT_OUT_ATTRS = ['onclick', '@click', 'x-on:click', ':href', 'x-bind:href',
    'data-preview-file', 'data-preview-doc', 'data-receipt-preview', 'data-modal-open',
    'data-dialog-target', 'data-open-modal', 'data-tab-target', 'data-sidebar-toggle',
    'data-drawer-toggle', 'data-sidebar-mobile-toggle', 'data-drawer-close', 'data-no-boost'];
  var FORM_OPT_OUT_ATTRS = ['onsubmit', '@submit', 'x-on:submit', ':action', 'x-bind:action',
    ':method', 'x-bind:method', 'data-no-boost'];

  // -- htmx configuration ----------------------------------------------------
  // No page HTML in localStorage; back/forward does a normal load instead.
  htmx.config.historyCacheSize = 0;
  htmx.config.refreshOnHistoryMiss = true;
  htmx.config.scrollIntoViewOnBoost = false;
  // CSP nonces are per request. Scripts arriving in a swap must carry the
  // nonce of the document that is already loaded, not the one in the response.
  var nonceSource = document.querySelector('script[nonce]');
  if (nonceSource && nonceSource.nonce) {
    htmx.config.inlineScriptNonce = nonceSource.nonce;
  }

  function hasAnyAttr(el, names) {
    var own = el.getAttributeNames();
    for (var i = 0; i < own.length; i++) {
      for (var j = 0; j < names.length; j++) {
        if (own[i] === names[j] || own[i].indexOf(names[j] + '.') === 0) return true;
      }
    }
    return false;
  }

  function pathOf(raw) {
    try { return new URL(raw, window.location.href); } catch (_) { return null; }
  }

  function skipLink(a) {
    if (a.hasAttribute('download')) return true;
    if (a.target && a.target !== '_self') return true;
    if (hasAnyAttr(a, LINK_OPT_OUT_ATTRS)) return true;
    var u = pathOf(a.getAttribute('href') || '');
    if (!u || u.origin !== window.location.origin) return true;
    if (FULL_LOAD_PATH.test(u.pathname) || DOWNLOAD_PATH.test(u.pathname)) return true;
    if (u.pathname.indexOf('/documents/') !== -1 && /\/view$/.test(u.pathname)) return true;
    if (u.pathname.indexOf('/receipts/') !== -1 || u.pathname.indexOf('/uploads/') !== -1) return true;
    return false;
  }

  function skipForm(f) {
    if ((f.getAttribute('enctype') || '').toLowerCase() === 'multipart/form-data') return true;
    if ((f.getAttribute('method') || '').toLowerCase() === 'dialog') return true;
    var target = f.getAttribute('target');
    if (target && target !== '_self') return true;
    if (hasAnyAttr(f, FORM_OPT_OUT_ATTRS)) return true;
    if (f.querySelector('[formaction],[formmethod],[formenctype],[formtarget]')) return true;
    var u = pathOf(f.getAttribute('action') || window.location.href);
    if (!u || u.origin !== window.location.origin) return true;
    if (FULL_LOAD_PATH.test(u.pathname) || DOWNLOAD_PATH.test(u.pathname)) return true;
    return false;
  }

  // -- 1. Opt elements out before htmx wires them ------------------------------
  // htmx 1.9 ignores preventDefault() from inline/Alpine handlers and captures a
  // form's action once, so anything that relies on either must stay native.
  document.addEventListener('htmx:beforeProcessNode', function (evt) {
    var el = evt.target;
    if (!el || !el.tagName || el.hasAttribute('hx-boost')) return;
    if (!el.closest('[hx-boost="true"]')) return;
    if ((el.tagName === 'A' && skipLink(el)) || (el.tagName === 'FORM' && skipForm(el))) {
      el.setAttribute('hx-boost', 'false');
    }
  });

  // -- 2. Progress state -------------------------------------------------------
  var navFrom = null;

  function endNav() {
    document.documentElement.classList.remove('is-navigating');
  }

  document.addEventListener('htmx:beforeRequest', function (evt) {
    if (!evt.detail || !evt.detail.boosted) return;
    navFrom = window.location.pathname;
    document.documentElement.classList.add('is-navigating');
  });
  document.addEventListener('htmx:afterRequest', function (evt) {
    if (evt.detail && evt.detail.boosted) endNav();
  });
  document.addEventListener('htmx:sendError', endNav);
  document.addEventListener('htmx:timeout', endNav);

  // -- 3. Decide what a boosted response does ----------------------------------
  document.addEventListener('htmx:beforeSwap', function (evt) {
    var d = evt.detail;
    if (!d || !d.boosted || !d.xhr) return;
    var xhr = d.xhr;
    var verb = (d.requestConfig && d.requestConfig.verb) || 'get';
    var requested = pathOf((d.pathInfo && d.pathInfo.finalRequestPath) || window.location.href);
    var landed = pathOf(xhr.responseURL || '') || requested;
    var redirected = !!(requested && landed && requested.href !== landed.href);

    function fullLoad() {
      d.shouldSwap = false;
      endNav();
      if (verb === 'get' || redirected) {
        window.location.assign(landed.href);
      } else {
        // A POST that was not redirected has already run on the server;
        // resubmitting would run it twice. Show the resulting state instead.
        window.location.reload();
      }
    }

    if (landed && FULL_LOAD_PATH.test(landed.pathname)) return fullLoad();
    var type = (xhr.getResponseHeader('Content-Type') || '').toLowerCase();
    var disposition = (xhr.getResponseHeader('Content-Disposition') || '').toLowerCase();
    if (xhr.status === 204 || type.indexOf('text/html') === -1 || disposition.indexOf('attachment') !== -1) {
      return fullLoad();
    }

    var html = xhr.responseText || '';
    if (html.indexOf(HARD_NAV_MARK) !== -1) return fullLoad();

    var current = document.getElementById(MAIN_ID);
    var doc = new DOMParser().parseFromString(html, 'text/html');
    var incoming = doc.getElementById(MAIN_ID);
    if (!current || !incoming ||
        incoming.getAttribute('data-shell') !== current.getAttribute('data-shell') ||
        doc.documentElement.getAttribute('lang') !== document.documentElement.getAttribute('lang')) {
      return fullLoad();
    }

    d.target = current;
    d.serverResponse = incoming.innerHTML;
    d.shouldSwap = true;
    d.isError = false; // error pages that render inside the shell are shown in place
    if (doc.title) document.title = doc.title;
  });

  // -- 4. After the swap -------------------------------------------------------
  document.addEventListener('htmx:afterSettle', function (evt) {
    if (!evt.detail || !evt.detail.boosted) return;
    endNav();
    if (navFrom !== null && navFrom !== window.location.pathname) {
      window.scrollTo({ top: 0, behavior: 'instant' });
      var main = document.getElementById(MAIN_ID);
      if (main) {
        main.setAttribute('tabindex', '-1');
        main.focus({ preventScroll: true });
      }
    }
    navFrom = null;
  });
})();
