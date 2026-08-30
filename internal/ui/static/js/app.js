// Dawa24 Frontend Application Script
// Resilient Vanilla JS Engine: Tabs, Modals, Steppers, Dropdowns, HTMX & Flash Notices.

// Global Cookie Helper
function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^|;\\s*)' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}

// Universal Scroll Position Retention for all Platform Actions and Form Submissions
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

// 0. Auto-inject _csrf token into all Native HTML form submissions
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

// 0.1 Global Fetch Interceptor for CSRF
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
  // 1. HTMX CSRF Attachment
  document.body.addEventListener('htmx:configRequest', (evt) => {
    const csrfToken = getCookie('dawa_csrf');
    if (csrfToken) {
      evt.detail.headers['X-CSRF-Token'] = csrfToken;
    }
  });

  // 2. HTMX Error Handling & Custom Events
  document.body.addEventListener('htmx:responseError', (evt) => {
    const status = evt.detail.xhr.status;
    console.error('Request failed:', status, evt.detail.xhr.responseText);
    showToast(messageForStatus(status), 'error');
  });

  document.body.addEventListener('htmx:sendError', () => {
    showToast('تعذر الاتصال بالخادم. تحقق من اتصالك بالإنترنت.', 'error');
  });

  // HTMX Custom Trigger Events: showToast & cartUpdated
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

  // 3. Resilient Tab Switcher (Works universally)
  initTabSystem();

  // 4. Resilient Registration Stepper
  initRegistrationStepper();

  // 5. Universal Modal System (Deposit, Withdraw, Quotes, Addresses, Tracking)
  initModalManager();

  // 6. Universal Dropdown Toggle Handler
  initDropdowns();

  // 7. Animated Sidebar Toggle
  initSidebarToggle();

  // 8. Theme System (Light / Dark / System Auto)
  initThemeSystem();

  // 9. Scroll Reveal (brand surfaces only)
  initScrollReveal();

  // 10. Universal Google Maps Interactive Engine
  initMapPickers();
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

function applyTheme(theme, save = true) {
  let effectiveTheme = theme;
  if (theme === 'system') {
    const prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
    effectiveTheme = prefersDark ? 'dark' : 'light';
  }
  document.documentElement.setAttribute('data-theme', effectiveTheme);
  if (save) {
    localStorage.setItem('dawa24-theme', theme);
  }

  // Update UI active buttons
  document.querySelectorAll('[data-set-theme]').forEach((btn) => {
    const btnTheme = btn.getAttribute('data-set-theme');
    if (btnTheme === theme) {
      btn.style.background = 'var(--accent-subtle)';
      btn.style.color = 'var(--accent-text)';
      btn.style.borderColor = 'var(--accent)';
      btn.style.fontWeight = '600';
    } else {
      btn.style.background = 'transparent';
      btn.style.color = 'var(--text-muted)';
      btn.style.borderColor = 'var(--border)';
      btn.style.fontWeight = '500';
    }
  });
}

// Universal Modal & Dialog Manager
function initModalManager() {
  document.addEventListener('click', (e) => {
    // Open trigger: data-modal-open, data-modal-target, data-dialog-target, data-open-modal
    const openBtn = e.target.closest('[data-modal-open], [data-modal-target], [data-dialog-target], [data-open-modal]');
    if (openBtn) {
      e.preventDefault();
      const modalId = openBtn.getAttribute('data-modal-open') ||
                      openBtn.getAttribute('data-modal-target') ||
                      openBtn.getAttribute('data-dialog-target') ||
                      openBtn.getAttribute('data-open-modal');
      window.openModal(modalId);
      return;
    }

    // Close trigger: data-modal-close, .modal-close, [data-dialog-close]
    const closeBtn = e.target.closest('[data-modal-close], .modal-close, [data-dialog-close], .btn-modal-close');
    if (closeBtn) {
      e.preventDefault();
      const targetId = closeBtn.getAttribute('data-modal-close') || closeBtn.getAttribute('data-dialog-close');
      if (targetId) {
        window.closeModal(targetId);
      } else {
        const parentModal = closeBtn.closest('.modal-overlay, dialog.modal, .modal, [id$="-modal"]');
        if (parentModal) {
          window.closeModal(parentModal.id);
        }
      }
      return;
    }

    // Click outside modal card to close
    if (e.target.classList.contains('modal-overlay')) {
      window.closeModal(e.target.id);
    }
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      document.querySelectorAll('.modal-overlay:not(.hidden)').forEach((m) => window.closeModal(m.id));
      document.querySelectorAll('dialog.modal[open]').forEach((d) => window.closeModal(d.id));
    }
  });
}

// Opening a modal used to stall for a beat on the heavier admin screens.
// Four things were happening at once, and all four are addressed here.
//
//   1. Every .glass-panel on the page carries backdrop-filter. A page can hold
//      dozens of them, and the overlay adds one more on top. backdrop-filter is
//      the most expensive property in this stylesheet -- each instance forces
//      its own compositing layer and re-blurs on every frame -- so the browser
//      was re-blurring the whole page behind a blur. The body class below turns
//      the background blurs off for as long as a modal is open; the overlay's
//      own blur is the only one that survives, which is the only one anybody
//      can see anyway.
//
//   2. Nothing locked the page scroll, so the content behind the overlay
//      scrolled with the wheel.
//
//   3. Every open dispatched a synthetic window resize 100ms later, which runs
//      every resize listener on the page -- charts, tables, the sidebar, Leaflet
//      -- whether or not the modal had anything to resize. It now runs only when
//      the modal actually contains a map.
//
//   4. Two modals open at once each restored the scroll on close. The open
//      count keeps the lock honest.
let openModalCount = 0;
let scrollLockY = 0;

window.openModal = function(id) {
  if (!id || typeof id !== 'string' || !id.trim()) return;
  const cleanId = id.trim();
  const el = document.getElementById(cleanId);
  if (!el) return;

  if (el.tagName === 'DIALOG' && typeof el.showModal === 'function') {
    try { el.showModal(); } catch (_) { el.setAttribute('open', ''); }
  } else {
    el.classList.remove('hidden');
    el.style.display = 'flex';
  }

  if (openModalCount === 0) {
    scrollLockY = window.scrollY || 0;
    document.body.classList.add('modal-open');
    document.body.style.top = `-${scrollLockY}px`;
  }
  openModalCount += 1;

  // Only pay for map initialisation when this modal actually holds a map.
  if (el.querySelector('[data-map-picker], .map-canvas, .leaflet-container')) {
    setTimeout(() => {
      initMapPickers();
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
    : !el.classList.contains('hidden');

  if (el.tagName === 'DIALOG' && typeof el.close === 'function') {
    try { el.close(); } catch (_) { el.removeAttribute('open'); }
  } else {
    el.classList.add('hidden');
    el.style.display = 'none';
  }

  // Only a modal that was actually open releases the lock, so a stray close
  // call cannot unlock the page while another modal is still showing.
  if (!wasOpen) return;
  openModalCount = Math.max(0, openModalCount - 1);
  if (openModalCount === 0) {
    document.body.classList.remove('modal-open');
    document.body.style.top = '';
    window.scrollTo(0, scrollLockY);
  }
};

window.openDialog = window.openModal;
window.closeDialog = window.closeModal;

// Animated Sidebar Toggle & Scroll Preservation Handler
function initSidebarToggle() {
  const isMobile = () => window.innerWidth <= 768;
  const isCollapsed = localStorage.getItem('dawa_sidebar_collapsed') === 'true';
  const sidebar = document.querySelector('.sidebar');
  if (sidebar && isCollapsed && !isMobile()) {
    sidebar.classList.add('collapsed');
  }

  let backdrop = document.querySelector('.sidebar-backdrop');
  if (!backdrop) {
    backdrop = document.createElement('div');
    backdrop.className = 'sidebar-backdrop';
    document.body.appendChild(backdrop);
    backdrop.addEventListener('click', () => {
      const sb = document.querySelector('.sidebar');
      if (sb) {
        sb.classList.remove('mobile-open');
        backdrop.classList.remove('active');
      }
    });
  }

  document.addEventListener('click', (e) => {
    const toggleBtn = e.target.closest('[data-sidebar-toggle]');
    if (toggleBtn) {
      e.preventDefault();
      const sb = document.querySelector('.sidebar');
      if (sb) {
        if (isMobile()) {
          const isOpen = sb.classList.toggle('mobile-open');
          if (backdrop) {
            backdrop.classList.toggle('active', isOpen);
          }
        } else {
          const collapsed = sb.classList.toggle('collapsed');
          localStorage.setItem('dawa_sidebar_collapsed', collapsed ? 'true' : 'false');
        }
      }
    }
  });

  initSidebarScrollPreservation();
}

function initSidebarScrollPreservation() {
  const nav = document.querySelector('.sidebar-nav') || document.querySelector('.sidebar');
  if (!nav) return;

  const storageKey = 'dawa_sidebar_scroll_top';
  const currentPath = window.location.pathname;

  // 1. Fallback URL path matching if server did not render an active class
  const links = Array.from(nav.querySelectorAll('.sidebar-link'));
  let activeLink = nav.querySelector('.sidebar-link.active');

  if (!activeLink && links.length > 0) {
    // Sort links by href length descending to match most specific sub-path first
    const sortedLinks = [...links].sort((a, b) => (b.getAttribute('href') || '').length - (a.getAttribute('href') || '').length);
    for (const l of sortedLinks) {
      const href = l.getAttribute('href');
      if (href && (currentPath === href || (href !== '/admin/dashboard' && currentPath.startsWith(href)))) {
        l.classList.add('active');
        activeLink = l;
        break;
      }
    }
  }

  // 2. Restore scroll position or scroll active item into view
  const savedScroll = sessionStorage.getItem(storageKey);
  if (savedScroll !== null) {
    nav.scrollTop = parseInt(savedScroll, 10);
  } else if (activeLink) {
    activeLink.scrollIntoView({ block: 'center', behavior: 'instant' });
  }

  // If active link is not visible even after savedScroll, adjust
  if (activeLink) {
    const rect = activeLink.getBoundingClientRect();
    const navRect = nav.getBoundingClientRect();
    if (rect.top < navRect.top || rect.bottom > navRect.bottom) {
      activeLink.scrollIntoView({ block: 'nearest', behavior: 'instant' });
    }
  }

  // 3. Save on scroll
  let scrollTimeout;
  nav.addEventListener('scroll', () => {
    clearTimeout(scrollTimeout);
    scrollTimeout = setTimeout(() => {
      sessionStorage.setItem(storageKey, nav.scrollTop.toString());
    }, 40);
  }, { passive: true });

  // 4. Save on click of any sidebar link
  document.addEventListener('click', (e) => {
    const link = e.target.closest('.sidebar-link, .sidebar a');
    if (link) {
      const currentNav = document.querySelector('.sidebar-nav') || document.querySelector('.sidebar');
      if (currentNav) {
        sessionStorage.setItem(storageKey, currentNav.scrollTop.toString());
      }
    }
  });

  // 5. Save before unload
  window.addEventListener('beforeunload', () => {
    const currentNav = document.querySelector('.sidebar-nav') || document.querySelector('.sidebar');
    if (currentNav) {
      sessionStorage.setItem(storageKey, currentNav.scrollTop.toString());
    }
  });

  // 6. Handle HTMX partial swaps if triggered
  document.body.addEventListener('htmx:afterSwap', () => {
    const currentNav = document.querySelector('.sidebar-nav') || document.querySelector('.sidebar');
    const scrollPos = sessionStorage.getItem(storageKey);
    if (currentNav && scrollPos !== null) {
      currentNav.scrollTop = parseInt(scrollPos, 10);
    }
  });
}

// Tab Switcher with URL hash and pushState support
function initTabSystem() {
  const tabLists = document.querySelectorAll('.tab-list');
  tabLists.forEach((list) => {
    const buttons = list.querySelectorAll('[data-tab-target]');
    buttons.forEach((btn) => {
      btn.addEventListener('click', (e) => {
        e.preventDefault();
        const targetId = btn.getAttribute('data-tab-target');
        switchTab(targetId, list);
      });
    });
  });

  // Auto-activate tab from URL query param or hash if present
  const urlTab = new URLSearchParams(window.location.search).get('tab');
  if (urlTab) {
    switchTab(urlTab);
  } else if (window.location.hash) {
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

  // Determine container scope to avoid breaking independent tabs on the same page
  const container = targetPane.closest('.tabs-container, .card, main, form, section') || document.body;
  const siblingPanes = container.querySelectorAll('.tab-pane');
  siblingPanes.forEach((pane) => pane.classList.remove('active'));

  // Scope button deactivation
  const btnList = specificList || (targetBtn ? targetBtn.closest('.tab-list') : null) || container;
  const siblingBtns = btnList.querySelectorAll('[data-tab-target]');
  siblingBtns.forEach((btn) => btn.classList.remove('active'));

  // Activate target smoothly
  targetPane.classList.add('active');
  if (targetBtn) targetBtn.classList.add('active');

  // Update hash without scrolling
  try {
    history.replaceState(null, null, '#' + cleanId);
  } catch (_) {}
}

// 3-Step Registration Onboarding Controller & Password Strength Meter
function initRegistrationStepper() {
  const form = document.getElementById('registration-onboarding-form');
  const typeCards = document.querySelectorAll('[data-account-type]');
  const hiddenInput = document.getElementById('reg-account-type-input');
  const badgeLabel = document.getElementById('reg-selected-badge');

  const step1 = document.getElementById('reg-step-1');
  const step2 = document.getElementById('reg-step-2');
  const step3 = document.getElementById('reg-step-3');

  const stepIndicator1 = document.getElementById('step-indicator-1');
  const stepIndicator2 = document.getElementById('step-indicator-2');
  const stepIndicator3 = document.getElementById('step-indicator-3');

  const gotoStep2Btn = document.getElementById('reg-goto-step-2');
  const backToStep1Btns = [document.getElementById('reg-back-to-step-1'), document.getElementById('reg-step-2-back')];
  const gotoStep3Btn = document.getElementById('reg-goto-step-3');
  const backToStep2Btns = [document.getElementById('reg-step-3-back'), document.getElementById('reg-step-3-back-btn')];

  if (!step1 || !step2) return;

  function showStep(stepNum) {
    [step1, step2, step3].forEach((s, idx) => {
      if (!s) return;
      s.style.display = (idx + 1 === stepNum) ? 'flex' : 'none';
      if (idx + 1 === stepNum) s.classList.add('active');
      else s.classList.remove('active');
    });

    const indicators = [stepIndicator1, stepIndicator2, stepIndicator3];
    indicators.forEach((ind, idx) => {
      if (!ind) return;
      const num = ind.querySelector('.step-num');
      if (idx + 1 === stepNum) {
        ind.style.color = 'var(--accent)';
        if (num) {
          num.style.background = 'var(--accent)';
          num.style.color = '#ffffff';
          num.style.border = 'none';
          num.textContent = (idx + 1).toString();
        }
      } else if (idx + 1 < stepNum) {
        ind.style.color = 'var(--success, #10b981)';
        if (num) {
          num.style.background = 'var(--success, #10b981)';
          num.style.color = '#ffffff';
          num.style.border = 'none';
          num.textContent = '✓';
        }
      } else {
        ind.style.color = 'var(--text-muted)';
        if (num) {
          num.style.background = 'var(--surface-raised)';
          num.style.color = 'var(--text-secondary)';
          num.style.border = '1px solid var(--border)';
          num.textContent = (idx + 1).toString();
        }
      }
    });

    window.scrollTo({ top: 0, behavior: 'smooth' });
    // Invalidate Leaflet maps on step change with multiple safety timeouts
    if (typeof L !== 'undefined') {
      [50, 150, 300, 600].forEach((delay) => {
        setTimeout(() => {
          document.querySelectorAll('[data-map-picker] .map-canvas, [data-map-picker], .leaflet-container').forEach((c) => {
            if (c._leaflet_map) c._leaflet_map.invalidateSize();
          });
          window.dispatchEvent(new Event('resize'));
        }, delay);
      });
    }
  }

  // If returning with an error alert, auto-advance to step 3 so the user stays on their filled form
  const errorAlert = document.querySelector('.alert-danger');
  if (errorAlert && errorAlert.textContent.trim()) {
    showStep(3);
  }

  // Account Type Selection Cards
  typeCards.forEach((card) => {
    card.addEventListener('click', () => {
      const type = card.getAttribute('data-account-type');
      if (hiddenInput) hiddenInput.value = type;

      typeCards.forEach((c) => {
        const isThis = c === card;
        c.style.borderColor = isThis ? 'var(--accent)' : 'var(--border)';
        c.style.background = isThis ? 'var(--accent-subtle)' : 'var(--surface-raised)';
        const icon = c.querySelector('.type-icon');
        if (icon) {
          icon.style.background = isThis ? 'var(--accent)' : 'var(--surface-sunken)';
          icon.style.color = isThis ? '#ffffff' : 'var(--text-secondary)';
        }
        const check = c.querySelector('.type-check');
        if (check) {
          check.style.borderColor = isThis ? 'var(--accent)' : 'var(--border)';
          check.style.background = isThis ? 'var(--accent)' : 'transparent';
          check.style.color = isThis ? '#ffffff' : 'transparent';
        }
      });

      if (badgeLabel) {
        if (type === 'supplier' || type === 'vendor') badgeLabel.textContent = 'نوع الحساب: مورّد / شركة ومستودع أدوية';
        else if (type === 'job_seeker') badgeLabel.textContent = 'نوع الحساب: باحث عن عمل / كادر طبي وصيدلاني';
        else badgeLabel.textContent = 'نوع الحساب: صيدلية / منشأة طبية مرخصة';
      }

      document.querySelectorAll('[data-type-visibility]').forEach((el) => {
        const allowed = el.getAttribute('data-type-visibility').split(' ');
        const isVisible = allowed.includes(type);
        el.style.display = isVisible ? 'block' : 'none';
        const input = el.querySelector('input, select');
        if (input && el.hasAttribute('data-was-required')) {
          input.required = isVisible;
        }
      });
    });
  });

  if (gotoStep2Btn) gotoStep2Btn.addEventListener('click', () => showStep(2));
  backToStep1Btns.forEach((b) => { if (b) b.addEventListener('click', () => showStep(1)); });

  if (gotoStep3Btn) {
    gotoStep3Btn.addEventListener('click', () => {
      // Validate step 2 required fields
      const legalName = document.getElementById('reg-legal-name');
      const tradeAr = document.getElementById('reg-trade-ar');
      const cr = document.getElementById('reg-cr');
      const address = document.getElementById('reg-address');

      if (legalName && legalName.required && !legalName.value.trim()) {
        legalName.focus();
        showToast('يرجى كتابة الاسم القانوني للمنشأة.', 'warning');
        return;
      }
      if (tradeAr && tradeAr.required && !tradeAr.value.trim()) {
        tradeAr.focus();
        showToast('يرجى كتابة الاسم التجاري للمنشأة.', 'warning');
        return;
      }
      if (cr && cr.required && !cr.value.trim()) {
        cr.focus();
        showToast('يرجى كتابة رقم السجل التجاري.', 'warning');
        return;
      }
      if (address && address.required && !address.value.trim()) {
        address.focus();
        showToast('يرجى تحديد موقع المنشأة وكتابة العنوان التفصيلي.', 'warning');
        return;
      }

      showStep(3);
    });
  }

  backToStep2Btns.forEach((b) => { if (b) b.addEventListener('click', () => showStep(2)); });

  // Real-Time Password Strength Meter & Security Checklist
  const pwdInput = document.getElementById('reg-password');
  const togglePwdBtn = document.getElementById('reg-toggle-pwd');
  const strengthLabel = document.getElementById('pass-strength-label');
  const barSegments = document.querySelectorAll('#pwd-strength-bars .strength-bar-seg');
  const ruleLen = document.getElementById('rule-length');
  const ruleCase = document.getElementById('rule-casing');
  const ruleNum = document.getElementById('rule-number');
  const ruleSpec = document.getElementById('rule-special');

  if (togglePwdBtn && pwdInput) {
    togglePwdBtn.addEventListener('click', () => {
      const isPass = pwdInput.type === 'password';
      pwdInput.type = isPass ? 'text' : 'password';
      togglePwdBtn.textContent = isPass ? '🔒' : '👁️';
    });
  }

  if (pwdInput) {
    pwdInput.addEventListener('input', () => {
      const val = pwdInput.value;
      const hasLen = val.length >= 8;
      const hasUpper = /[A-Z]/.test(val);
      const hasLower = /[a-z]/.test(val);
      const hasCasing = hasUpper && hasLower;
      const hasNum = /[0-9]/.test(val);
      const hasSpec = /[^A-Za-z0-9]/.test(val);

      function updateRule(el, pass) {
        if (!el) return;
        const icon = el.querySelector('.rule-icon');
        if (pass) {
          el.style.color = 'var(--success, #10b981)';
          if (icon) { icon.textContent = '✓'; icon.style.color = 'var(--success, #10b981)'; }
        } else {
          el.style.color = 'var(--text-secondary)';
          if (icon) { icon.textContent = '●'; icon.style.color = 'var(--text-muted)'; }
        }
      }

      updateRule(ruleLen, hasLen);
      updateRule(ruleCase, hasCasing);
      updateRule(ruleNum, hasNum);
      updateRule(ruleSpec, hasSpec);

      let score = 0;
      if (hasLen) score++;
      if (hasCasing) score++;
      if (hasNum) score++;
      if (hasSpec) score++;

      const colors = ['#ef4444', '#f97316', '#eab308', '#10b981'];
      const labels = ['أدخل كلمة المرور', 'ضعيفة جداً 🔴', 'ضعيفة 🟠', 'متوسطة 🟡', 'قوية ومطابقة للمعايير 🟢'];

      if (val.length === 0) {
        if (strengthLabel) { strengthLabel.textContent = labels[0]; strengthLabel.style.color = 'var(--text-muted)'; }
        barSegments.forEach((b) => { b.style.background = 'var(--border)'; });
      } else {
        const color = colors[score - 1] || colors[0];
        if (strengthLabel) {
          strengthLabel.textContent = labels[score] || labels[1];
          strengthLabel.style.color = color;
        }
        barSegments.forEach((b, idx) => {
          b.style.background = idx < score ? color : 'var(--border)';
        });
      }
    });
  }

  // Form Submit Interceptor
  if (form) {
    form.addEventListener('submit', (e) => {
      const name = document.getElementById('reg-name');
      const email = document.getElementById('reg-email');
      const phone = document.getElementById('reg-phone');
      const password = document.getElementById('reg-password');

      if (name && !name.value.trim()) {
        e.preventDefault();
        name.focus();
        showToast('يرجى إدخال اسم المسؤول / الصيدلي المسؤول.', 'warning');
        return;
      }
      if (email && !email.value.trim()) {
        e.preventDefault();
        email.focus();
        showToast('يرجى إدخال البريد الإلكتروني الرسمي.', 'warning');
        return;
      }
      if (phone && !phone.value.trim()) {
        e.preventDefault();
        phone.focus();
        showToast('يرجى إدخال رقم الهاتف.', 'warning');
        return;
      }
      if (password) {
        const val = password.value;
        if (val.length < 8) {
          e.preventDefault();
          password.focus();
          showToast('كلمة المرور يجب أن لا تقل عن 8 أحرف.', 'warning');
          return;
        }
        const hasUpper = /[A-Z]/.test(val);
        const hasLower = /[a-z]/.test(val);
        const hasNum = /[0-9]/.test(val);
        const hasSpec = /[^A-Za-z0-9]/.test(val);
        if (!hasUpper || !hasLower || !hasNum || !hasSpec) {
          e.preventDefault();
          password.focus();
          showToast('كلمة المرور يجب أن تحتوي على أحرف كبيرة وصغيرة وأرقام ورموز خاصة (@, $, !).', 'warning');
          return;
        }
      }
    });
  }
}

// Live Reverse Geocoding via OpenStreetMap Nominatim
let reverseGeocodeTimer = null;
function fetchDetailedAddressFromCoords(lat, lon) {
  clearTimeout(reverseGeocodeTimer);
  reverseGeocodeTimer = setTimeout(async () => {
    try {
      const addressInput = document.getElementById('reg-address') || document.querySelector('input[name="address"]');
      const hint = document.getElementById('reg-address-hint');
      if (!addressInput) return;

      if (hint) hint.textContent = '⏳ جلب العنوان بالتفصيل...';
      const resp = await fetch(`https://nominatim.openstreetmap.org/reverse?format=json&lat=${lat}&lon=${lon}&zoom=18&addressdetails=1&accept-language=ar`, {
        headers: { 'Accept': 'application/json' }
      });
      if (!resp.ok) {
        if (hint) hint.textContent = '📍 تم تحديد الإحداثيات بنجاح';
        return;
      }
      const data = await resp.json();
      if (data && data.address) {
        const a = data.address;
        const parts = [];
        if (a.road || a.street) parts.push(a.road || a.street);
        if (a.neighbourhood || a.suburb || a.quarter) parts.push(a.neighbourhood || a.suburb || a.quarter);
        if (a.city || a.town || a.village || a.county) parts.push(a.city || a.town || a.village || a.county);
        if (a.state) parts.push(a.state);

        const fullAddr = parts.filter(Boolean).join('، ');
        if (fullAddr) {
          addressInput.value = fullAddr;
          if (hint) hint.textContent = '📍 تم تحديث العنوان تلقائياً من الخريطة';
        }
      }
    } catch (e) {
      console.warn('Reverse geocoding error:', e);
    }
  }, 400);
}

// Egyptian Cities Coordinates Reference Table
const EGYPT_CITIES_COORDS = [
  { name: 'القاهرة', lat: 30.0444, lon: 31.2357 },
  { name: 'القاهرة الجديدة', lat: 30.0131, lon: 31.2089 },
  { name: 'الشروق', lat: 30.1219, lon: 31.3665 },
  { name: 'مدينة بدر', lat: 30.1842, lon: 31.2482 },
  { name: 'الجيزة', lat: 30.0131, lon: 31.2089 },
  { name: 'مدينة ستة أكتوبر', lat: 30.0648, lon: 30.9706 },
  { name: 'الشيخ زايد', lat: 30.1111, lon: 30.8544 },
  { name: 'الحوامدية', lat: 29.9667, lon: 31.3000 },
  { name: 'أوسيم', lat: 29.8833, lon: 31.2333 },
  { name: 'البدرشين', lat: 29.8167, lon: 31.2833 },
  { name: 'الإسكندرية', lat: 31.2001, lon: 29.9187 },
  { name: 'برج العرب', lat: 31.0333, lon: 29.7667 },
  { name: 'مدينة برج العرب الجديدة', lat: 30.9164, lon: 29.5553 },
  { name: 'شبرا الخيمة', lat: 30.4500, lon: 31.1833 },
  { name: 'الخصوص', lat: 30.4667, lon: 31.1833 },
  { name: 'بنها', lat: 30.4667, lon: 31.1833 },
  { name: 'قليوب', lat: 30.1833, lon: 31.2167 },
  { name: 'العبور', lat: 30.2000, lon: 31.3167 },
  { name: 'بور سعيد', lat: 31.2654, lon: 32.3020 },
  { name: 'بور فؤاد', lat: 31.2333, lon: 32.3167 },
  { name: 'السويس', lat: 29.9668, lon: 32.5498 },
  { name: 'الإسماعيلية', lat: 30.5965, lon: 32.2715 },
  { name: 'فايد', lat: 30.3333, lon: 32.3000 },
  { name: 'القنطرة شرق', lat: 30.8333, lon: 32.3167 },
  { name: 'طنطا', lat: 30.7865, lon: 31.0004 },
  { name: 'المحلة الكبرى', lat: 30.9667, lon: 31.1667 },
  { name: 'كفر الزيات', lat: 30.8167, lon: 30.8167 },
  { name: 'زفتى', lat: 30.7167, lon: 31.2500 },
  { name: 'المنصورة', lat: 31.0409, lon: 31.3785 },
  { name: 'ميت غمر', lat: 30.7167, lon: 31.2500 },
  { name: 'السنبلاوين', lat: 30.8833, lon: 31.4667 },
  { name: 'دكرنس', lat: 31.0833, lon: 31.6000 },
  { name: 'بلقاس', lat: 31.2333, lon: 31.3667 },
  { name: 'طلخا', lat: 31.0500, lon: 31.3667 },
  { name: 'الزقازيق', lat: 30.5877, lon: 31.5020 },
  { name: 'العاشر من رمضان', lat: 30.3000, lon: 31.7333 },
  { name: 'بلبيس', lat: 30.4167, lon: 31.5667 },
  { name: 'فاقوس', lat: 30.7333, lon: 31.8000 },
  { name: 'منيا القمح', lat: 30.5167, lon: 31.3500 },
  { name: 'أبو حماد', lat: 30.5500, lon: 31.6833 },
  { name: 'أبو كبير', lat: 30.7333, lon: 31.6667 },
  { name: 'دمنهور', lat: 31.0409, lon: 30.4667 },
  { name: 'كفر الدوار', lat: 31.1333, lon: 30.1333 },
  { name: 'إدكو', lat: 31.3000, lon: 30.3000 },
  { name: 'رشيد', lat: 31.4000, lon: 30.4167 },
  { name: 'شبين الكوم', lat: 30.5522, lon: 31.0094 },
  { name: 'منوف', lat: 30.4667, lon: 30.9333 },
  { name: 'أشمون', lat: 30.3000, lon: 30.9833 },
  { name: 'قويسنا', lat: 30.5667, lon: 31.1500 },
  { name: 'مدينة السادات', lat: 30.3833, lon: 30.5167 },
  { name: 'كفر الشيخ', lat: 31.1107, lon: 30.9388 },
  { name: 'دسوق', lat: 31.1333, lon: 30.6500 },
  { name: 'فوه', lat: 31.2000, lon: 30.5500 },
  { name: 'دمياط', lat: 31.4165, lon: 31.8133 },
  { name: 'دمياط الجديدة', lat: 31.4333, lon: 31.6667 },
  { name: 'رأس البر', lat: 31.5167, lon: 31.8167 },
  { name: 'الفيوم', lat: 29.3084, lon: 30.8428 },
  { name: 'بني سويف', lat: 29.0661, lon: 31.0994 },
  { name: 'المنيا', lat: 28.1099, lon: 30.7503 },
  { name: 'ملوي', lat: 27.7333, lon: 30.8333 },
  { name: 'أسيوط', lat: 27.1801, lon: 31.1837 },
  { name: 'ديروط', lat: 27.5667, lon: 30.8167 },
  { name: 'سوهاج', lat: 26.5569, lon: 31.6948 },
  { name: 'طهطا', lat: 26.7667, lon: 31.5000 },
  { name: 'جرجا', lat: 26.3333, lon: 31.8833 },
  { name: 'قنا', lat: 26.1642, lon: 32.7267 },
  { name: 'نجع حمادي', lat: 26.0500, lon: 32.2500 },
  { name: 'الأقصر', lat: 25.6872, lon: 32.6396 },
  { name: 'إسنا', lat: 25.2833, lon: 32.5500 },
  { name: 'أسوان', lat: 24.0889, lon: 32.8998 },
  { name: 'إدفو', lat: 24.9833, lon: 32.8667 },
  { name: 'كوم أمبو', lat: 24.4667, lon: 32.9500 },
  { name: 'مطروح', lat: 31.3525, lon: 27.2453 },
  { name: 'العلمين', lat: 30.8333, lon: 28.9500 },
  { name: 'سيوة', lat: 29.2000, lon: 25.5167 },
  { name: 'الغردقة', lat: 27.2579, lon: 33.8116 },
  { name: 'سفاجا', lat: 26.7292, lon: 33.9365 },
  { name: 'مرسى علم', lat: 25.0676, lon: 34.8966 },
  { name: 'الخارجة', lat: 25.4390, lon: 30.5586 },
  { name: 'الداخلة', lat: 25.5167, lon: 28.9667 },
  { name: 'العريش', lat: 31.1316, lon: 33.7984 },
  { name: 'الطور', lat: 28.2410, lon: 33.6230 },
  { name: 'شرم الشيخ', lat: 27.9158, lon: 34.3299 },
  { name: 'دهب', lat: 28.5094, lon: 34.5137 },
  { name: 'نويبع', lat: 29.0436, lon: 34.6644 },
  { name: 'طابا', lat: 29.4925, lon: 34.8967 },
  { name: 'رأس سدر', lat: 29.5892, lon: 32.7144 }
];

function findClosestEgyptianCity(lat, lon) {
  let closest = null;
  let minDist = Infinity;
  for (const c of EGYPT_CITIES_COORDS) {
    const d = Math.hypot(lat - c.lat, lon - c.lon);
    if (d < minDist) {
      minDist = d;
      closest = c;
    }
  }
  return closest;
}

function syncCityDropdownsWithCoordinates(lat, lon) {
  const closest = findClosestEgyptianCity(lat, lon);
  if (!closest) return null;

  // 1. Sync MapPicker toolbar city select if present
  const mapCitySelects = document.querySelectorAll('[data-city-selector]');
  mapCitySelects.forEach((selectEl) => {
    let bestOpt = null;
    let minOptDist = Infinity;
    for (let i = 0; i < selectEl.options.length; i++) {
      const opt = selectEl.options[i];
      if (!opt.value) continue;
      const [optLat, optLon] = opt.value.split(',').map((v) => parseFloat(v.trim()));
      if (!isNaN(optLat) && !isNaN(optLon)) {
        const d = Math.hypot(lat - optLat, lon - optLon);
        if (d < minOptDist) {
          minOptDist = d;
          bestOpt = opt;
        }
      }
    }
    if (bestOpt && minOptDist < 0.45) {
      selectEl.value = bestOpt.value;
      // Sync hidden city ID
      const cityId = bestOpt.dataset.cityId;
      if (cityId) {
        document.querySelectorAll('[data-map-city-id], input[name="branch_city_id"], input[name="city_id"]').forEach((hi) => {
          hi.value = cityId;
        });
      }
    }
  });

  return closest;
}


// Universal Dropdown Toggle
function initDropdowns() {
  document.addEventListener('click', (e) => {
    const dropdownBtn = e.target.closest('.dropdown > button, .dropdown > .btn, [data-dropdown-toggle]');
    const clickedDropdown = dropdownBtn ? dropdownBtn.closest('.dropdown') : null;

    // Close all other dropdowns
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


// Scroll Reveal: IntersectionObserver for .reveal elements
function initScrollReveal() {
  var targets = document.querySelectorAll('.reveal');
  if (!targets.length) return;

  // If no IntersectionObserver or reduced motion, show everything immediately
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

// Leaflet is loaded on demand, not in the document head.
//
// It was previously requested by every page in the platform -- roughly 200 KB
// of CSS, JavaScript and marker images -- for the two screens that draw a map.
// ensureLeaflet injects it the first time a map element is actually present and
// resolves immediately on every call after that.
let leafletPromise = null;
function ensureLeaflet() {
  if (typeof L !== 'undefined') return Promise.resolve();
  if (leafletPromise) return leafletPromise;

  leafletPromise = new Promise(function (resolve, reject) {
    if (!document.querySelector('link[data-leaflet-css]')) {
      const css = document.createElement('link');
      css.rel = 'stylesheet';
      css.href = '/static/vendor/leaflet/leaflet.css';
      css.setAttribute('data-leaflet-css', '');
      document.head.appendChild(css);
    }
    const script = document.createElement('script');
    script.src = '/static/vendor/leaflet/leaflet.js';
    script.onload = function () { resolve(); };
    script.onerror = function () {
      leafletPromise = null;
      reject(new Error('leaflet failed to load'));
    };
    document.head.appendChild(script);
  });
  return leafletPromise;
}

// Universal Leaflet & Interactive Map Engine
function initMapPickers() {
  const containers = document.querySelectorAll('[data-map-picker]');
  if (!containers.length) return;

  if (typeof L === 'undefined') {
    ensureLeaflet().then(initMapPickers).catch(function (err) {
      // A map that cannot load says so, rather than leaving an empty box.
      containers.forEach(function (container) {
        if (container.dataset.mapInitialized === 'true') return;
        container.setAttribute('data-map-failed', 'true');
      });
      console.error('map picker:', err);
    });
    return;
  }

  containers.forEach((container) => {
    if (container.dataset.mapInitialized === 'true') return;

    const canvas = container.querySelector('.map-canvas, .map-container, [data-map-canvas], .leaflet-map-canvas') || container;
    if (!canvas) return;

    const parentScope = container.closest('form') || container.closest('.modal-card') || container.closest('.glass-panel') || container.closest('.card') || document;
    const latInput = container.querySelector('[data-map-lat], [data-map-input="lat"], input[name="latitude"], input[name="branch_lat"], input[name="city_lat"], input[name="gov_lat"]') || parentScope.querySelector('[data-map-input="lat"], input[name="latitude"], input[name="branch_lat"], input[name="city_lat"], input[name="gov_lat"]');
    const lonInput = container.querySelector('[data-map-lon], [data-map-input="lon"], input[name="longitude"], input[name="branch_lon"], input[name="city_lon"], input[name="gov_lon"]') || parentScope.querySelector('[data-map-input="lon"], input[name="longitude"], input[name="branch_lon"], input[name="city_lon"], input[name="gov_lon"]');
    const radiusInput = container.querySelector('[data-map-radius], [data-map-input="radius"], input[name="radius"]') || parentScope.querySelector('[data-map-radius], [data-map-input="radius"]');
    const gmapsInput = container.querySelector('[data-map-google-url], [data-map-input="google_url"], input[name="google_maps_url"], input[name="branch_google_maps_url"]') || parentScope.querySelector('[data-map-input="google_url"], input[name="google_maps_url"], input[name="branch_google_maps_url"]');
    const badge = container.querySelector('[data-map-badge], [data-map-coords-badge]') || parentScope.querySelector('[data-map-coords-badge]');
    const gmapsLinks = container.querySelectorAll('[data-google-maps-link]');
    const citySelect = container.querySelector('[data-city-selector], [data-map-city], select[name="city_id"], select[name="governorate_id"]') || parentScope.querySelector('[data-map-city], select[name="governorate_id"]');
    const locateBtn = container.querySelector('[data-locate-me-btn], [data-map-locate], .btn-locate') || parentScope.querySelector('[data-map-locate]');

    let initialLat = parseFloat(canvas.dataset.lat || container.dataset.defaultLat || (latInput ? latInput.value : '30.0444')) || 30.0444;
    let initialLon = parseFloat(canvas.dataset.lon || container.dataset.defaultLon || (lonInput ? lonInput.value : '31.2357')) || 31.2357;
    let initialRadius = parseInt(canvas.dataset.radius || container.dataset.defaultRadius || (radiusInput ? radiusInput.value : '1000'), 10) || 1000;

    container.dataset.mapInitialized = 'true';

    // Initialize Leaflet Map
    const map = L.map(canvas, {
      center: [initialLat, initialLon],
      zoom: 13,
      zoomControl: true,
      scrollWheelZoom: true,
    });

    canvas._leaflet_map = map;
    container._leaflet_map = map;

    // Add standard OpenStreetMap tiles
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 19,
      attribution: '&copy; OpenStreetMap contributors',
    }).addTo(map);

    // Auto resize observer on canvas container
    if (window.ResizeObserver) {
      const ro = new ResizeObserver(() => {
        map.invalidateSize();
      });
      ro.observe(canvas);
      ro.observe(container);
    }
    if (window.IntersectionObserver) {
      const io = new IntersectionObserver((entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            setTimeout(() => map.invalidateSize(), 50);
            setTimeout(() => map.invalidateSize(), 200);
          }
        });
      });
      io.observe(canvas);
    }

    // Custom pulse marker icon (matches Laravel reference)
    const customIcon = L.divIcon({
      className: 'custom-map-pin',
      html: `<div style="width:36px; height:36px; display:flex; align-items:center; justify-content:center; background:#0ea5e9; color:#fff; border-radius:50%; box-shadow:0 4px 14px rgba(14,165,233,0.5); border:3px solid #ffffff; font-size:18px; cursor:grab; transform:translate(-50%, -50%);">📍</div>`,
      iconSize: [36, 36],
      iconAnchor: [18, 18],
    });

    const marker = L.marker([initialLat, initialLon], {
      draggable: true,
      icon: customIcon,
    }).addTo(map);

    let circle = null;
    if (radiusInput || canvas.dataset.radius) {
      circle = L.circle([initialLat, initialLon], {
        radius: initialRadius,
        color: '#0ea5e9',
        fillColor: '#0ea5e9',
        fillOpacity: 0.18,
        weight: 2,
      }).addTo(map);
    }

    function updateCoordinates(lat, lon, zoom = null) {
      const fixedLat = parseFloat(lat.toFixed(6));
      const fixedLon = parseFloat(lon.toFixed(6));

      marker.setLatLng([fixedLat, fixedLon]);
      if (circle) circle.setLatLng([fixedLat, fixedLon]);

      if (zoom !== null) {
        map.setView([fixedLat, fixedLon], zoom, { animate: true });
      } else {
        map.panTo([fixedLat, fixedLon], { animate: true });
      }

      if (latInput) {
        latInput.value = fixedLat.toFixed(6);
        latInput.dispatchEvent(new Event('input', { bubbles: true }));
      }
      if (lonInput) {
        lonInput.value = fixedLon.toFixed(6);
        lonInput.dispatchEvent(new Event('input', { bubbles: true }));
      }

      const gmapsUrl = `https://www.google.com/maps?q=${fixedLat},${fixedLon}`;
      if (gmapsInput) gmapsInput.value = gmapsUrl;
      gmapsLinks.forEach((link) => { link.href = gmapsUrl; });
      if (badge) badge.textContent = `${fixedLat.toFixed(4)}, ${fixedLon.toFixed(4)}`;

      // Sync city selectors and hidden city ID
      syncCityDropdownsWithCoordinates(fixedLat, fixedLon);

      // Auto-fetch detailed street/district address via Reverse Geocoding
      fetchDetailedAddressFromCoords(fixedLat, fixedLon);
    }

    container._updateCoords = updateCoordinates;
    canvas._updateCoords = updateCoordinates;

    // Map Click Handler
    map.on('click', (e) => {
      updateCoordinates(e.latlng.lat, e.latlng.lng);
    });

    // Marker Drag Handler
    marker.on('dragend', () => {
      const pos = marker.getLatLng();
      updateCoordinates(pos.lat, pos.lng);
    });

    // Manual Lat / Lon Input Change Handlers
    if (latInput && lonInput) {
      const onManualInputChange = () => {
        const parsedLat = parseFloat(latInput.value);
        const parsedLon = parseFloat(lonInput.value);
        if (!isNaN(parsedLat) && !isNaN(parsedLon)) {
          updateCoordinates(parsedLat, parsedLon, map.getZoom());
        }
      };
      latInput.addEventListener('change', onManualInputChange);
      lonInput.addEventListener('change', onManualInputChange);
    }

    // Radius Input & City Preset & Dropdown Selector (Auto-Pan & Zoom)
    const citySelectors = parentScope.querySelectorAll('select[name="city_id"], select[name="branch_city_id"], select[name="governorate_id"], [data-city-selector], [data-map-city], [data-gov-selector]');
    citySelectors.forEach((sel) => {
      sel.addEventListener('change', (e) => {
        const selEl = e.target;
        const opt = selEl.selectedOptions && selEl.selectedOptions[0] ? selEl.selectedOptions[0] : null;
        if (!opt) return;

        let cLat = parseFloat(opt.dataset.lat || opt.getAttribute('data-lat'));
        let cLon = parseFloat(opt.dataset.lng || opt.getAttribute('data-lng') || opt.dataset.lon || opt.getAttribute('data-lon'));

        if (isNaN(cLat) || isNaN(cLon)) {
          const val = selEl.value;
          if (val && val.includes(',')) {
            const parts = val.split(',').map((v) => parseFloat(v.trim()));
            cLat = parts[0];
            cLon = parts[1];
          }
        }

        if (!isNaN(cLat) && !isNaN(cLon) && (cLat !== 0 || cLon !== 0)) {
          updateCoordinates(cLat, cLon, 13);
          const name = opt.text ? opt.text.trim() : '';
          if (window.showToast && name && !name.startsWith('--')) {
            showToast(`تم تحريك الخريطة تلقائياً إلى: ${name} 📍`, 'info');
          }
        }
      });
    });

    if (radiusInput && circle) {
      radiusInput.addEventListener('input', () => {
        const rad = parseInt(radiusInput.value, 10);
        if (!isNaN(rad) && rad > 0) {
          circle.setRadius(rad);
        }
      });
    }

    // Google Maps URL Paste & Input Auto-Extractor
    if (gmapsInput) {
      const onGmapsUrlInput = () => {
        const val = gmapsInput.value.trim();
        if (!val) return;
        
        const qMatch = val.match(/[?&]q=([-+]?\d*\.?\d+)[,\s]+([-+]?\d*\.?\d+)/i);
        const atMatch = val.match(/@([-+]?\d*\.?\d+)[,\s]+([-+]?\d*\.?\d+)/);
        const llMatch = val.match(/[?&]ll=([-+]?\d*\.?\d+)[,\s]+([-+]?\d*\.?\d+)/i);
        const rawMatch = val.match(/^([-+]?\d*\.?\d+)[,\s]+([-+]?\d*\.?\d+)$/);

        let match = qMatch || atMatch || llMatch || rawMatch;
        if (match) {
          const parsedLat = parseFloat(match[1]);
          const parsedLon = parseFloat(match[2]);
          if (!isNaN(parsedLat) && !isNaN(parsedLon)) {
            updateCoordinates(parsedLat, parsedLon, 16);
            showToast('تم استخراج وتحديث الإحداثيات تلقائياً من الرابط 📍', 'success');
          }
        }
      };
      gmapsInput.addEventListener('input', onGmapsUrlInput);
      gmapsInput.addEventListener('paste', () => setTimeout(onGmapsUrlInput, 50));
    }

    // GPS Locate Me Button (High Accuracy)
    if (locateBtn) {
      locateBtn.addEventListener('click', (e) => {
        e.preventDefault();
        if (!navigator.geolocation) {
          showToast('خاصية تحديد الموقع غير مدعومة في متصفحك.', 'warning');
          return;
        }

        locateBtn.disabled = true;
        locateBtn.innerHTML = '<span>⏳ جارٍ التحديد بدقة...</span>';

        navigator.geolocation.getCurrentPosition(
          (pos) => {
            locateBtn.disabled = false;
            locateBtn.innerHTML = '<span>📍 موقعي الحالي</span>';
            const userLat = pos.coords.latitude;
            const userLon = pos.coords.longitude;
            updateCoordinates(userLat, userLon, 16);
            const nearest = syncCityDropdownsWithCoordinates(userLat, userLon);
            if (nearest) {
              showToast(`تم تحديد موقعك بدقة عالية وتحديث المحافظة التابعة إلى: ${nearest.name} 📍`, 'success');
            } else {
              showToast('تم تحديد موقعك الجغرافي بدقة.', 'success');
            }
          },
          (err) => {
            locateBtn.disabled = false;
            locateBtn.innerHTML = '<span>📍 موقعي الحالي</span>';
            console.warn('Geolocation error:', err.message);
            showToast('تعذر جلب موقع GPS. يرجى تفعيل إذن الوصول للموقع في المتصفح.', 'warning');
          },
          { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 }
        );
      });
    }

    // Auto Invalidate Size for Modals and Tabs
    const modalParent = container.closest('.modal-overlay, .modal-backdrop, dialog, .tab-pane');
    if (modalParent) {
      const resizeObserver = new MutationObserver(() => {
        if (getComputedStyle(modalParent).display !== 'none' || modalParent.hasAttribute('open') || modalParent.classList.contains('active')) {
          setTimeout(() => map.invalidateSize(), 150);
          setTimeout(() => map.invalidateSize(), 350);
        }
      });
      resizeObserver.observe(modalParent, { attributes: true, attributeFilter: ['style', 'class', 'open'] });
    }

    window.addEventListener('resize', () => map.invalidateSize());

    setTimeout(() => map.invalidateSize(), 150);
    setTimeout(() => map.invalidateSize(), 400);
  });
}

// Import progress polling.
//
// The catalogue import prepares in the background because AI enrichment turns a
// large file into minutes of work. This keeps the panel honest while that runs
// and reloads into the review the moment it finishes, so nobody has to know to
// press refresh.
function initImportProgress() {
  const panel = document.getElementById('import-progress');
  if (!panel) return;

  const sessionID = panel.dataset.session;
  if (!sessionID) return;

  const fill = document.getElementById('import-progress-fill');
  const message = document.getElementById('import-progress-message');
  const count = document.getElementById('import-progress-count');

  // Two seconds is faster than the phases change and slow enough that a
  // half-hour import is a few hundred requests, not tens of thousands.
  const POLL_MS = 2000;
  let stopped = false;

  async function poll() {
    if (stopped) return;
    try {
      const res = await fetch(`/admin/products/import/${sessionID}/progress`, {
        headers: { 'Accept': 'application/json' },
        cache: 'no-store',
      });
      if (!res.ok) throw new Error(`progress ${res.status}`);
      const data = await res.json();

      if (message && data.message) message.textContent = data.message;
      if (count) {
        count.textContent = data.total > 0
          ? `${data.current.toLocaleString('en')} / ${data.total.toLocaleString('en')}`
          : '';
      }
      if (fill) {
        if (typeof data.percent === 'number' && data.percent >= 0) {
          fill.classList.remove('is-indeterminate');
          fill.style.width = `${data.percent}%`;
        } else {
          fill.classList.add('is-indeterminate');
          fill.style.width = '';
        }
      }

      if (data.done) {
        stopped = true;
        // Reload rather than patch the DOM: the finished page is a different
        // page — structure, counts, per-row table — and the server already
        // knows how to render it.
        window.location.reload();
        return;
      }
    } catch (err) {
      // A dropped poll is not worth surfacing; the next one will catch up. Only
      // a run that never reports again leaves the panel where it was, and the
      // page explains that the results appear automatically.
      console.debug('import progress poll failed', err);
    }
    window.setTimeout(poll, POLL_MS);
  }

  window.setTimeout(poll, POLL_MS);
}

// Show the chosen file's name in the upload drop zone, so the admin can see
// what they picked before committing to a long run.
function initImportDropZone() {
  const input = document.getElementById('import-file-input');
  const title = document.getElementById('import-drop-title');
  const drop = document.getElementById('import-drop');
  if (!input || !title || !drop) return;

  input.addEventListener('change', () => {
    const file = input.files && input.files[0];
    title.textContent = file ? file.name : 'اختر الملف أو اسحبه إلى هنا';
  });

  ['dragenter', 'dragover'].forEach((evt) => {
    drop.addEventListener(evt, (e) => {
      e.preventDefault();
      drop.classList.add('is-over');
    });
  });
  ['dragleave', 'drop'].forEach((evt) => {
    drop.addEventListener(evt, (e) => {
      e.preventDefault();
      drop.classList.remove('is-over');
    });
  });
  drop.addEventListener('drop', (e) => {
    if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length) {
      input.files = e.dataTransfer.files;
      input.dispatchEvent(new Event('change'));
    }
  });
}

document.addEventListener('DOMContentLoaded', () => {
  initImportProgress();
  initImportDropZone();
});
