// boost-nav.js -- in-place navigation for the dashboard and public shells.
//
// hx-boost sits on .app-shell (dashboards) and on the public shell's header,
// body and footer. Every boosted response is reduced to the inner
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
  var FULL_LOAD_PATH = /^\/(auth|api|lang)\/|\/logout$/;
  // Click/submit handlers (onclick, @click, onsubmit="return confirm()",
  // @submit.prevent) no longer opt an element out: guardHandlers() makes htmx
  // honour their preventDefault(). Bound URLs (:href, :action) are re-read at
  // request time in htmx:configRequest. What is left here are elements whose
  // click is owned by a document-level delegate that runs after htmx.
  var LINK_OPT_OUT_ATTRS = [
    'data-preview-file', 'data-preview-doc', 'data-receipt-preview', 'data-modal-open',
    'data-dialog-target', 'data-open-modal', 'data-tab-target', 'data-sidebar-toggle',
    'data-drawer-toggle', 'data-sidebar-mobile-toggle', 'data-drawer-close', 'data-no-boost'];
  // A bound method would change which parameters htmx sends; keep those native.
  var FORM_OPT_OUT_ATTRS = [':method', 'x-bind:method', 'data-no-boost'];

  // -- htmx configuration ----------------------------------------------------
  // No page HTML in localStorage; back/forward does a normal load instead.
  htmx.config.historyCacheSize = 0;
  htmx.config.refreshOnHistoryMiss = true;
  htmx.config.scrollIntoViewOnBoost = false;
  // htmx "settles" class/style from an old element onto a new one with the same
  // id and then restores the server's values ~20ms later. Alpine has already
  // hidden the new element with x-show (style="display: none") by then, so the
  // restore strips that style and the notification panel, the account menu and
  // every other id'd x-show element pops open on navigation. Alpine owns
  // class and style here; htmx must not touch them.
  htmx.config.attributesToSettle = [];
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
    if ((f.getAttribute('enctype') || '').toLowerCase() === 'multipart/form-data' && !f.hasAttribute('data-boost-upload')) return true;
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

  // htmx 1.9 calls preventDefault() itself and never asks whether someone else
  // already did, so `onsubmit="return confirm(...)"` answered with Cancel was
  // submitted anyway. This listener is added before htmx wires the element, so
  // it runs after inline and Alpine handlers (registered earlier) and before
  // htmx: a cancelled event stops here.
  function guardHandlers(el, type) {
    if (el.__dawaBoostGuard) return;
    el.__dawaBoostGuard = true;
    el.addEventListener(type, function (e) {
      if (!e.defaultPrevented) return;
      e.stopImmediatePropagation();
      // app.js's double-submit lock runs in the capture phase, before the
      // confirm() above was answered. A cancelled submit must not leave the
      // form locked for 8 seconds; a real in-flight one keeps its lock.
      if (type === 'submit' && !el.__dawaInFlight) unlockForm(el);
    });
  }

  function unlockForm(form) {
    if (form.dataset) delete form.dataset.submitting;
    setTimeout(function () {
      // app.js adds .is-busy 20ms after the submit event.
      Array.prototype.forEach.call(form.querySelectorAll('.is-busy'), function (b) {
        b.classList.remove('is-busy');
        b.removeAttribute('aria-busy');
      });
    }, 30);
  }

  // -- 1. Opt elements out before htmx wires them ------------------------------
  document.addEventListener('htmx:beforeProcessNode', function (evt) {
    var el = evt.target;
    if (!el || !el.tagName || el.hasAttribute('hx-boost')) return;
    if (!el.closest('[hx-boost="true"]')) return;
    if (el.tagName === 'A') {
      if (skipLink(el)) el.setAttribute('hx-boost', 'false');
      else guardHandlers(el, 'click');
    } else if (el.tagName === 'FORM') {
      if (skipForm(el)) el.setAttribute('hx-boost', 'false');
      else guardHandlers(el, 'submit');
    }
  });

  // htmx reads href/action once, when it wires the element. Alpine may have
  // changed it since (:action="'/admin/users/' + id + '/edit'"), so use the
  // value the element carries at the moment of the request.
  document.addEventListener('htmx:configRequest', function (evt) {
    var d = evt.detail;
    var el = evt.target;
    if (!d || !d.boosted || !el || !el.getAttribute) return;
    var current = el.tagName === 'A' ? el.getAttribute('href')
      : el.tagName === 'FORM' ? el.getAttribute('action') : null;
    if (!current) return;
    var u = pathOf(current);
    if (!u || u.origin !== window.location.origin) {
      evt.preventDefault();
      return;
    }
    if (FULL_LOAD_PATH.test(u.pathname) || DOWNLOAD_PATH.test(u.pathname)) {
      // The bound value now points somewhere that must not be swapped.
      evt.preventDefault();
      if (el.tagName === 'A') window.location.assign(u.href);
      else nativeSubmit.call(el);
      return;
    }
    d.path = u.pathname + u.search + u.hash;
  });

  // -- 1b. Programmatic form.submit() ------------------------------------------
  // `onchange="this.form.submit()"` (filters, per-row status selects) never
  // fires a submit event, so htmx never sees it. Route boosted forms through
  // htmx; everything else keeps the native behaviour. app.js wraps this again to
  // add the CSRF field, and its wrapper runs first.
  var nativeSubmit = HTMLFormElement.prototype.submit;
  HTMLFormElement.prototype.submit = function () {
    var data = this['htmx-internal-data'];
    if (!data || !data.boosted || this.getAttribute('hx-boost') === 'false') {
      return nativeSubmit.call(this);
    }
    var verb = (this.getAttribute('method') || 'get').toLowerCase();
    var action = this.getAttribute('action') || (window.location.pathname + window.location.search);
    // form.submit() skips constraint validation; so does this.
    var hadNoValidate = this.noValidate;
    this.noValidate = true;
    try {
      htmx.ajax(verb, action, { source: this });
    } finally {
      this.noValidate = hadNoValidate;
    }
  };

  // -- 1c. HX-Redirect after an hx-post action ---------------------------------
  // redirectWithNotice answers an htmx action (approve, save, delete buttons)
  // with HX-Redirect, which htmx turns into a hard page load. Replay it as a
  // boosted click instead, so it goes through the same swap-or-fall-back rules
  // as any other in-place navigation.
  var REDIRECT_MARK = 'data-dawa-redirect';

  document.addEventListener('htmx:beforeOnLoad', function (evt) {
    var d = evt.detail;
    if (!d || d.boosted || !d.xhr) return;
    var loc = d.xhr.getResponseHeader('HX-Redirect');
    if (!loc || d.xhr.getResponseHeader('HX-Refresh') === 'true') return;
    var u = pathOf(loc);
    if (!u || u.origin !== window.location.origin) return;
    if (FULL_LOAD_PATH.test(u.pathname) || DOWNLOAD_PATH.test(u.pathname)) return;
    var main = document.getElementById(MAIN_ID);
    var host = main && main.closest('[hx-boost="true"]');
    if (!host) return;
    evt.preventDefault();
    var a = document.createElement('a');
    a.href = u.pathname + u.search + u.hash;
    a.hidden = true;
    a.setAttribute(REDIRECT_MARK, '');
    // Outside <main>, so the swap does not detach it mid-request.
    host.appendChild(a);
    htmx.process(a);
    a.click();
  });

  document.addEventListener('htmx:afterOnLoad', function (evt) {
    var el = evt.detail && evt.detail.elt;
    if (el && el.hasAttribute && el.hasAttribute(REDIRECT_MARK)) el.remove();
  });
  document.addEventListener('htmx:sendError', function (evt) {
    var el = evt.detail && evt.detail.elt;
    if (el && el.hasAttribute && el.hasAttribute(REDIRECT_MARK)) el.remove();
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
    if (evt.detail.elt && evt.detail.elt.tagName === 'FORM') evt.detail.elt.__dawaInFlight = true;
  });
  document.addEventListener('htmx:afterRequest', function (evt) {
    if (!evt.detail || !evt.detail.boosted) return;
    endNav();
    // app.js locks a submitted form for 8s against double submits. That suits a
    // page that is about to unload; a boosted form that stays on screen (an
    // error, a form outside <main>) must be usable again at once.
    var el = evt.detail.elt;
    if (el && el.tagName === 'FORM') {
      el.__dawaInFlight = false;
      unlockForm(el);
    }
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

    // A dialog removed while open never fires "close", so app.js would keep
    // body.modal-open (scroll lock) forever. Close them before they go.
    Array.prototype.forEach.call(current.querySelectorAll('dialog[open]'), function (dlg) {
      try { dlg.close(); } catch (_) {}
    });

    d.target = current;
    d.serverResponse = incoming.innerHTML;
    d.shouldSwap = true;
    d.isError = false; // error pages that render inside the shell are shown in place
    if (doc.title) document.title = doc.title;
  });

  // The public header lives outside <main>, so its active rail item is not
  // re-rendered. Mirror layouts.IsCurrentPath: exact path, trailing slash ignored.
  function syncPublicNav() {
    var path = window.location.pathname.replace(/(.)\/$/, '$1');
    Array.prototype.forEach.call(document.querySelectorAll('.nav-root--public .nav-link'), function (a) {
      var current = a.getAttribute('href') === path;
      a.classList.toggle('is-active', current);
      if (current) a.setAttribute('aria-current', 'page');
      else a.removeAttribute('aria-current');
    });
  }

  // -- 4. After the swap -------------------------------------------------------
  document.addEventListener('htmx:afterSettle', function (evt) {
    if (!evt.detail || !evt.detail.boosted) return;
    endNav();
    syncPublicNav();
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
