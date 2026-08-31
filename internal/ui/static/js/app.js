// Dawa24 Frontend Application Script
// Resilient Vanilla JS Engine: Tabs, Modals, Dropdowns, HTMX & Flash Notices.

// Global Cookie Helper
function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^|;\\s*)' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}

// Universal Scroll Position Retention for Platform Actions and Form Submissions
(function() {
  function saveScroll() {
    try {
      if (window.scrollY > 0) {
        sessionStorage.setItem('dawa24_scroll_pos', JSON.stringify({
          y: window.scrollY,
          path: window.location.pathname,
          time: Date.now()
        }));
      }
    } catch(e) {}
  }

  function restoreScroll() {
    try {
      var raw = sessionStorage.getItem('dawa24_scroll_pos');
      if (!raw) return;
      var data = JSON.parse(raw);
      sessionStorage.removeItem('dawa24_scroll_pos');
      if (data && typeof data.y === 'number' && (Date.now() - data.time < 20000)) {
        window.scrollTo({ top: data.y, behavior: 'instant' });
        setTimeout(function() {
          if (Math.abs(window.scrollY - data.y) > 10) {
            window.scrollTo({ top: data.y, behavior: 'instant' });
          }
        }, 50);
        setTimeout(function() {
          if (Math.abs(window.scrollY - data.y) > 10) {
            window.scrollTo({ top: data.y, behavior: 'instant' });
          }
        }, 150);
      }
    } catch(e) {}
  }

  document.addEventListener('submit', function(e) {
    saveScroll();
  }, true);

  document.addEventListener('click', function(e) {
    var btn = e.target.closest('button[type="submit"], a.btn, [data-keep-scroll]');
    if (btn) {
      saveScroll();
    }
  }, true);

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', restoreScroll);
  } else {
    restoreScroll();
  }
  window.addEventListener('pageshow', restoreScroll);

  document.addEventListener('htmx:beforeRequest', saveScroll);
  document.addEventListener('htmx:afterSettle', restoreScroll);
})();

// Auto-inject _csrf token into all Native HTML form submissions
document.addEventListener('submit', (e) => {
  const form = e.target;
  if (form && form.tagName === 'FORM') {
    const method = (form.getAttribute('method') || 'GET').toUpperCase();
    if (method !== 'GET') {
      const csrfToken = getCookie('dawa_csrf');
      if (csrfToken) {
        let input = form.querySelector('input[name="_csrf"]');
        if (!input) {
          input = document.createElement('input');
          input.type = 'hidden';
          input.name = '_csrf';
          form.appendChild(input);
        }
        input.value = csrfToken;
      }
    }
  }
}, true);

// Global Fetch Interceptor for CSRF
if (typeof window.fetch === 'function') {
  const _origFetch = window.fetch;
  window.fetch = function(url, options = {}) {
    const method = (options.method || 'GET').toUpperCase();
    if (method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS') {
      const csrfToken = getCookie('dawa_csrf');
      if (csrfToken) {
        if (!options.headers) {
          options.headers = { 'X-CSRF-Token': csrfToken };
        } else if (options.headers instanceof Headers) {
          if (!options.headers.has('X-CSRF-Token')) {
            options.headers.set('X-CSRF-Token', csrfToken);
          }
        } else if (Array.isArray(options.headers)) {
          options.headers.push(['X-CSRF-Token', csrfToken]);
        } else if (typeof options.headers === 'object') {
          if (!options.headers['X-CSRF-Token'] && !options.headers['x-csrf-token']) {
            options.headers['X-CSRF-Token'] = csrfToken;
          }
        }
      }
    }
    return _origFetch.call(this, url, options);
  };
}

document.addEventListener('DOMContentLoaded', () => {
  // HTMX CSRF Attachment
  document.body.addEventListener('htmx:configRequest', (evt) => {
    const csrfToken = getCookie('dawa_csrf');
    if (csrfToken) {
      evt.detail.headers['X-CSRF-Token'] = csrfToken;
    }
  });

  // HTMX Error Handling & Custom Events
  document.body.addEventListener('htmx:responseError', (evt) => {
    const status = evt.detail.xhr.status;
    console.error('Request failed:', status, evt.detail.xhr.responseText);
    showToast(messageForStatus(status), 'error');
  });

  document.body.addEventListener('htmx:sendError', () => {
    showToast('تعذر الاتصال بالخادم. تحقق من اتصالك بالإنترنت.', 'error');
  });

  document.body.addEventListener('showToast', (evt) => {
    const detail = evt.detail || {};
    const msg = detail.message || detail.value || 'تمت العملية بنجاح';
    const type = detail.type || 'info';
    showToast(msg, type);
  });

  document.body.addEventListener('cartUpdated', (evt) => {
    const detail = evt.detail || {};
    const count = detail.count;
    if (typeof count === 'number') {
      document.querySelectorAll('.cart-badge, .cart-count, [data-cart-count]').forEach((el) => {
        el.textContent = count > 0 ? count.toString() : '';
        el.style.display = count > 0 ? 'inline-flex' : 'none';
      });
    }
  });

  showNoticeFromQuery();
  initTabSystem();
  initModalManager();
  initDropdowns();
  initSidebarToggle();
  initThemeSystem();
  initScrollReveal();
});

// Theme Management System
function initThemeSystem() {
  // The URL parameter wins over the stored preference, and it has to be read
  // here as well as in the inline script in base.templ. That script sets
  // data-theme before first paint; this function runs a moment later and used
  // to reset it from localStorage alone, silently undoing a ?theme= link.
  //
  // The visual regression harness varies theme through the URL, so every
  // baseline it captured came out in the browser's default theme -- four of the
  // eight were duplicates of the other four and the suite could not see a
  // light-mode regression at all. A shared link carrying ?theme= was equally
  // broken for real users, just less visibly.
  //
  // The URL value is deliberately not persisted: it styles this view, it does
  // not change what the reader chose.
  const urlTheme = new URLSearchParams(window.location.search).get('theme');
  const stored = localStorage.getItem('dawa24-theme') || 'system';
  const currentTheme = (urlTheme === 'light' || urlTheme === 'dark') ? urlTheme : stored;
  applyTheme(currentTheme, false);

  document.addEventListener('click', (e) => {
    const themeBtn = e.target.closest('[data-set-theme]');
    if (themeBtn) {
      e.preventDefault();
      const theme = themeBtn.getAttribute('data-set-theme');
      applyTheme(theme, true);
    }
  });

  if (window.matchMedia) {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if ((localStorage.getItem('dawa24-theme') || 'system') === 'system') {
        applyTheme('system', false);
      }
    });
  }
}

function applyTheme(theme, persist = false) {
  if (persist) {
    localStorage.setItem('dawa24-theme', theme);
  }
  let isDark = false;
  if (theme === 'dark') {
    isDark = true;
  } else if (theme === 'light') {
    isDark = false;
  } else {
    isDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  }

  document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light');

  document.querySelectorAll('[data-set-theme]').forEach((btn) => {
    const btnTheme = btn.getAttribute('data-set-theme');
    btn.classList.toggle('active', btnTheme === theme);
  });
}

// Sidebar Collapse, Drawer & Navigation Shell
function initSidebarToggle() {
  const sidebar = document.getElementById('app-sidebar') || document.querySelector('.sidebar');
  if (!sidebar) return;

  // Persisted Desktop Collapsed State (above 1024px)
  const savedState = localStorage.getItem('dawa24-sidebar-collapsed');
  if (savedState === 'true' && window.innerWidth >= 1024) {
    sidebar.classList.add('collapsed');
    document.body.classList.add('sidebar-collapsed');
  }

  // Ensure Backdrop element exists
  let backdrop = document.querySelector('.sidebar-backdrop');
  if (!backdrop) {
    backdrop = document.createElement('div');
    backdrop.className = 'sidebar-backdrop';
    document.body.appendChild(backdrop);
  }

  let lastDrawerOpener = null;

  function setDrawerOpen(isOpen, openerBtn) {
    sidebar.classList.toggle('mobile-open', isOpen);
    backdrop.classList.toggle('active', isOpen);
    document.body.classList.toggle('sidebar-mobile-open', isOpen);

    if (openerBtn) {
      lastDrawerOpener = openerBtn;
    }

    const toggles = document.querySelectorAll('[data-drawer-toggle], [data-sidebar-mobile-toggle]');
    toggles.forEach((btn) => {
      btn.setAttribute('aria-expanded', isOpen ? 'true' : 'false');
    });

    if (isOpen) {
      // Focus first interactive element in sidebar
      const firstFocusable = sidebar.querySelector('a, button, input, [tabindex]:not([tabindex="-1"])');
      if (firstFocusable) {
        setTimeout(() => firstFocusable.focus(), 50);
      }
    } else if (lastDrawerOpener && typeof lastDrawerOpener.focus === 'function') {
      lastDrawerOpener.focus();
    }
  }

  document.addEventListener('click', (e) => {
    // Desktop Collapse Toggle
    const toggleBtn = e.target.closest('[data-sidebar-toggle]');
    if (toggleBtn) {
      e.preventDefault();
      const isCollapsed = sidebar.classList.toggle('collapsed');
      document.body.classList.toggle('sidebar-collapsed', isCollapsed);
      localStorage.setItem('dawa24-sidebar-collapsed', isCollapsed ? 'true' : 'false');
      return;
    }

    // Mobile Off-canvas Drawer Toggle
    const drawerToggle = e.target.closest('[data-drawer-toggle], [data-sidebar-mobile-toggle]');
    if (drawerToggle) {
      e.preventDefault();
      const isCurrentlyOpen = sidebar.classList.contains('mobile-open');
      setDrawerOpen(!isCurrentlyOpen, drawerToggle);
      return;
    }

    // Backdrop Click Dismissal
    if (e.target === backdrop) {
      setDrawerOpen(false);
    }
  });

  // Escape to close mobile drawer & Focus Trap
  document.addEventListener('keydown', (e) => {
    if (sidebar.classList.contains('mobile-open')) {
      if (e.key === 'Escape') {
        e.preventDefault();
        setDrawerOpen(false);
        return;
      }

      if (e.key === 'Tab') {
        const focusables = Array.from(sidebar.querySelectorAll('a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])'));
        if (focusables.length === 0) return;
        const first = focusables[0];
        const last = focusables[focusables.length - 1];

        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    }
  });

  initSidebarNav();
}

// Active Link Resolution & Scroll Position Preservation
function initSidebarNav() {
  const nav = document.querySelector('.sidebar-nav');
  if (!nav) return;

  try {
    const currentPath = window.location.pathname;
    const links = Array.from(nav.querySelectorAll('.sidebar-link'));
    const hasActive = links.some((l) => l.classList.contains('active'));
    if (!hasActive && links.length > 0) {
      const sorted = links.slice().sort((a, b) => {
        return (b.getAttribute('href') || '').length - (a.getAttribute('href') || '').length;
      });
      for (let i = 0; i < sorted.length; i++) {
        const href = sorted[i].getAttribute('href');
        if (href && (currentPath === href || (href !== '/admin/dashboard' && href !== '/vendor' && href !== '/customer' && currentPath.indexOf(href) === 0))) {
          sorted[i].classList.add('active');
          break;
        }
      }
    }

    const saved = sessionStorage.getItem('dawa_sidebar_scroll_top');
    if (saved !== null) {
      nav.scrollTop = parseInt(saved, 10);
    } else {
      const act = nav.querySelector('.sidebar-link.active');
      if (act) {
        act.scrollIntoView({ block: 'nearest' });
      }
    }

    nav.addEventListener('scroll', () => {
      sessionStorage.setItem('dawa_sidebar_scroll_top', nav.scrollTop);
    }, { passive: true });
  } catch (e) {}
}

// Universal Accessible Native Modal Manager
let openDialogCount = 0;
let scrollLockY = 0;
const lastActiveElements = new WeakMap();

function initModalManager() {
  // Delegate Trigger Opens
  document.addEventListener('click', (e) => {
    const openBtn = e.target.closest('[data-modal-open], [data-dialog-target], [data-open-modal]');
    if (openBtn) {
      e.preventDefault();
      const targetId = openBtn.getAttribute('data-modal-open') || openBtn.getAttribute('data-dialog-target') || openBtn.getAttribute('data-open-modal');
      if (!targetId) return;
      const dialog = document.getElementById(targetId.trim());
      if (dialog && typeof dialog.showModal === 'function') {
        lastActiveElements.set(dialog, openBtn);
        dialog.showModal();
      }
      return;
    }

    // Delegate Trigger Closes
    const closeBtn = e.target.closest('[data-modal-close], [data-close-modal], .modal-close');
    if (closeBtn) {
      const dialog = closeBtn.closest('dialog');
      if (dialog && typeof dialog.close === 'function') {
        e.preventDefault();
        dialog.close();
      }
      return;
    }

    // Backdrop Click on Native Dialog
    if (e.target.tagName === 'DIALOG' && e.target.classList.contains('modal')) {
      const rect = e.target.getBoundingClientRect();
      const isOutside = (
        e.clientX < rect.left ||
        e.clientX > rect.right ||
        e.clientY < rect.top ||
        e.clientY > rect.bottom ||
        e.target === e.currentTarget
      );
      // If click was directly on dialog backdrop (not on modal-box child)
      if (e.target === e.currentTarget && !e.target.querySelector('.modal-box')?.contains(e.explicitOriginalTarget || e.target)) {
        if (typeof e.target.close === 'function') {
          e.target.close();
        }
      }
    }
  });

  // Listen for native dialog open and close events for scroll locking and focus restoration
  document.addEventListener('close', (e) => {
    if (e.target.tagName === 'DIALOG') {
      handleDialogClose(e.target);
    }
  }, true);

  // Body scroll lock on dialog show
  const originalShowModal = HTMLDialogElement.prototype.showModal;
  HTMLDialogElement.prototype.showModal = function() {
    if (openDialogCount === 0) {
      scrollLockY = window.scrollY;
      document.body.classList.add('modal-open');
      document.body.style.top = `-${scrollLockY}px`;
    }
    openDialogCount += 1;

    // Trigger map invalidation if dialog contains map
    if (this.querySelector('[data-map-picker], .map-canvas, .leaflet-container')) {
      setTimeout(() => {
        if (typeof initMapPickers === 'function') initMapPickers();
        window.dispatchEvent(new Event('resize'));
      }, 100);
    }

    return originalShowModal.apply(this, arguments);
  };
}

function handleDialogClose(dialog) {
  if (openDialogCount > 0) {
    openDialogCount -= 1;
    if (openDialogCount === 0) {
      document.body.classList.remove('modal-open');
      document.body.style.top = '';
      window.scrollTo(0, scrollLockY);
    }
  }

  const opener = lastActiveElements.get(dialog);
  if (opener && typeof opener.focus === 'function') {
    opener.focus();
    lastActiveElements.delete(dialog);
  }
}

// Resilient Vanilla Tab System
function initTabSystem() {
  document.addEventListener('click', (e) => {
    const tabBtn = e.target.closest('[data-tab-target]');
    if (!tabBtn) return;

    e.preventDefault();
    const targetId = tabBtn.getAttribute('data-tab-target');
    const tabList = tabBtn.closest('.tab-list, .tabs, .nav-tabs, .pill-track, .sp-tabs-nav, .cb-tabs-nav');
    switchTab(targetId, tabList);
  });

  if (window.location.hash) {
    const hash = window.location.hash.substring(1);
    switchTab(hash);
  }
}

function switchTab(tabId, specificList) {
  if (!tabId || typeof tabId !== 'string') return;

  const cleanId = tabId.trim();
  const targetPane = document.getElementById('tab-' + cleanId) || document.querySelector(`[data-tab-pane="${cleanId}"]`);
  const targetBtn = document.querySelector(`[data-tab-target="${cleanId}"]`);

  if (!targetPane) return;

  const container = targetPane.closest('.tabs-container, .card, main, form, section') || document.body;
  const siblingPanes = container.querySelectorAll('.tab-pane');
  siblingPanes.forEach((pane) => pane.classList.remove('active'));

  const btnList = specificList || (targetBtn ? targetBtn.closest('.tab-list') : null) || container;
  const siblingBtns = btnList.querySelectorAll('[data-tab-target]');
  siblingBtns.forEach((btn) => btn.classList.remove('active'));

  targetPane.classList.add('active');
  if (targetBtn) targetBtn.classList.add('active');

  try {
    history.replaceState(null, null, '#' + cleanId);
  } catch (_) {}
}

// Universal Dropdown Toggle
function initDropdowns() {
  document.addEventListener('click', (e) => {
    const dropdownBtn = e.target.closest('.dropdown > button, .dropdown > .btn, [data-dropdown-toggle]');
    const clickedDropdown = dropdownBtn ? dropdownBtn.closest('.dropdown') : null;

    document.querySelectorAll('.dropdown.open, .dropdown.is-active').forEach((d) => {
      if (d !== clickedDropdown) {
        d.classList.remove('open', 'is-active');
        const menu = d.querySelector('.dropdown-menu');
        if (menu) menu.classList.remove('is-active');
      }
    });

    if (clickedDropdown) {
      e.stopPropagation();
      const isOpen = clickedDropdown.classList.toggle('open');
      clickedDropdown.classList.toggle('is-active', isOpen);
      const menu = clickedDropdown.querySelector('.dropdown-menu');
      if (menu) {
        menu.classList.toggle('is-active', isOpen);
      }
    }
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      document.querySelectorAll('.dropdown.open, .dropdown.is-active').forEach((d) => {
        d.classList.remove('open', 'is-active');
        const menu = d.querySelector('.dropdown-menu');
        if (menu) menu.classList.remove('is-active');
      });
    }
  });
}

// Flash Query Parameter Toast
function showNoticeFromQuery() {
  const params = new URLSearchParams(window.location.search);
  const kind = params.get('notice');
  const msg = params.get('msg');
  if (!kind || !msg) return;

  showToast(msg, kind === 'error' ? 'error' : 'success');

  params.delete('notice');
  params.delete('msg');
  const qs = params.toString();
  window.history.replaceState({}, '', window.location.pathname + (qs ? '?' + qs : ''));
}

function messageForStatus(status) {
  switch (status) {
    case 401: return 'انتهت الجلسة. يرجى تسجيل الدخول مرة أخرى.';
    case 403: return 'ليس لديك صلاحية لتنفيذ هذا الإجراء.';
    case 404: return 'العنصر المطلوب غير موجود.';
    case 409: return 'تعارض في البيانات المدخلة.';
    case 422: return 'بيانات غير صحيحة، يرجى مراجعة الحقول.';
    case 500: return 'حدث خطأ في الخادم. يرجى المحاولة لاحقاً.';
    default: return 'حدث خطأ غير متوقع. يرجى المحاولة مرة أخرى.';
  }
}

function showToast(message, type = 'info') {
  let container = document.getElementById('toast-container');
  if (!container) {
    container = document.createElement('div');
    container.id = 'toast-container';
    container.className = 'toast-container';
    document.body.appendChild(container);
  }

  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  
  const text = document.createElement('span');
  text.textContent = message;
  text.style.flex = '1';
  toast.appendChild(text);

  const closeBtn = document.createElement('button');
  closeBtn.type = 'button';
  closeBtn.className = 'toast-close';
  closeBtn.innerHTML = '&times;';
  closeBtn.onclick = () => {
    toast.remove();
  };
  toast.appendChild(closeBtn);

  container.appendChild(toast);

  setTimeout(() => {
    toast.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(-10px)';
    setTimeout(() => toast.remove(), 300);
  }, 4500);
}

// Scroll Reveal
function initScrollReveal() {
  var targets = document.querySelectorAll('.reveal');
  if (!targets.length) return;

  var prefersReduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (!('IntersectionObserver' in window) || prefersReduced) {
    targets.forEach(function(el) { el.classList.add('visible'); });
    return;
  }

  var observer = new IntersectionObserver(function(entries) {
    entries.forEach(function(entry) {
      if (entry.isIntersecting) {
        entry.target.classList.add('visible');
        observer.unobserve(entry.target);
      }
    });
  }, { threshold: 0.2 });

  targets.forEach(function(el) { observer.observe(el); });
}
