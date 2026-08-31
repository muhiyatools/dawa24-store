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
  const currentTheme = localStorage.getItem('dawa24-theme') || 'system';
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

// Sidebar Collapse & Mobile Drawer
function initSidebarToggle() {
  const sidebar = document.querySelector('.sidebar');
  if (!sidebar) return;

  const savedState = localStorage.getItem('dawa24-sidebar-collapsed');
  if (savedState === 'true' && window.innerWidth >= 1024) {
    sidebar.classList.add('collapsed');
    document.body.classList.add('sidebar-collapsed');
  }

  document.addEventListener('click', (e) => {
    const toggleBtn = e.target.closest('[data-sidebar-toggle]');
    if (toggleBtn) {
      e.preventDefault();
      const isCollapsed = sidebar.classList.toggle('collapsed');
      document.body.classList.toggle('sidebar-collapsed', isCollapsed);
      localStorage.setItem('dawa24-sidebar-collapsed', isCollapsed ? 'true' : 'false');
    }

    const mobileToggle = e.target.closest('[data-sidebar-mobile-toggle]');
    if (mobileToggle) {
      e.preventDefault();
      sidebar.classList.toggle('mobile-open');
      document.body.classList.toggle('sidebar-mobile-open');
    }
  });
}

// Universal Accessible Modal Manager
let openModalCount = 0;
let scrollLockY = 0;

function initModalManager() {
  document.addEventListener('click', (e) => {
    const openBtn = e.target.closest('[data-modal-open], [data-open-modal]');
    if (openBtn) {
      e.preventDefault();
      const targetId = openBtn.getAttribute('data-modal-open') || openBtn.getAttribute('data-open-modal');
      window.openModal(targetId);
      return;
    }

    const closeBtn = e.target.closest('[data-modal-close], [data-close-modal], .modal-close');
    if (closeBtn) {
      e.preventDefault();
      const modal = closeBtn.closest('dialog, .modal, .modal-backdrop, .glass-panel');
      if (modal && modal.id) {
        window.closeModal(modal.id);
      }
      return;
    }

    if (e.target.classList.contains('modal-backdrop') || e.target.classList.contains('modal')) {
      if (e.target.id) {
        window.closeModal(e.target.id);
      }
    }
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && openModalCount > 0) {
      const topModal = document.querySelector('dialog[open], .modal.open, .modal.is-active, .modal-backdrop.open');
      if (topModal && topModal.id) {
        window.closeModal(topModal.id);
      }
    }
  });
}

window.openModal = function(id) {
  if (!id || typeof id !== 'string' || !id.trim()) return;
  const cleanId = id.trim();
  const el = document.getElementById(cleanId);
  if (!el) return;

  if (el.tagName === 'DIALOG') {
    if (typeof el.showModal === 'function') {
      el.showModal();
    } else {
      el.setAttribute('open', '');
    }
  } else {
    el.classList.add('open', 'is-active');
  }

  if (openModalCount === 0) {
    scrollLockY = window.scrollY;
    document.body.classList.add('modal-open');
    document.body.style.top = `-${scrollLockY}px`;
  }
  openModalCount += 1;

  if (el.querySelector('[data-map-picker], .map-canvas, .leaflet-container')) {
    setTimeout(() => {
      if (typeof initMapPickers === 'function') initMapPickers();
      window.dispatchEvent(new Event('resize'));
    }, 100);
  }
};

window.closeModal = function(id) {
  if (!id || typeof id !== 'string' || !id.trim()) return;
  const cleanId = id.trim();
  const el = document.getElementById(cleanId);
  if (!el) return;

  const wasOpen = el.tagName === 'DIALOG'
    ? el.hasAttribute('open')
    : (el.classList.contains('open') || el.classList.contains('is-active'));

  if (el.tagName === 'DIALOG') {
    if (typeof el.close === 'function') {
      el.close();
    } else {
      el.removeAttribute('open');
    }
  } else {
    el.classList.remove('open', 'is-active');
  }

  if (wasOpen && openModalCount > 0) {
    openModalCount -= 1;
    if (openModalCount === 0) {
      document.body.classList.remove('modal-open');
      document.body.style.top = '';
      window.scrollTo(0, scrollLockY);
    }
  }
};

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
