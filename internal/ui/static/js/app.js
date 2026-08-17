// Dawa24 Frontend Application Script
// Resilient Vanilla JS Engine: Tabs, Modals, Steppers, Dropdowns, HTMX & Flash Notices.

document.addEventListener('DOMContentLoaded', () => {
  // 1. HTMX CSRF Attachment
  document.body.addEventListener('htmx:configRequest', (evt) => {
    const csrfToken = getCookie('dawa_csrf');
    if (csrfToken) {
      evt.detail.headers['X-CSRF-Token'] = csrfToken;
    }
  });

  // 2. HTMX Error Handling
  document.body.addEventListener('htmx:responseError', (evt) => {
    const status = evt.detail.xhr.status;
    console.error('Request failed:', status, evt.detail.xhr.responseText);
    showToast(messageForStatus(status), 'error');
  });

  document.body.addEventListener('htmx:sendError', () => {
    showToast('تعذر الاتصال بالخادم. تحقق من اتصالك بالإنترنت.', 'error');
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

window.openModal = function(id) {
  if (!id) return;
  const el = document.getElementById(id);
  if (!el) return;
  if (el.tagName === 'DIALOG' && typeof el.showModal === 'function') {
    try { el.showModal(); } catch (_) { el.setAttribute('open', ''); }
  } else {
    el.classList.remove('hidden');
    el.style.display = 'flex';
  }
};

window.closeModal = function(id) {
  if (!id) return;
  const el = document.getElementById(id);
  if (!el) return;
  if (el.tagName === 'DIALOG' && typeof el.close === 'function') {
    try { el.close(); } catch (_) { el.removeAttribute('open'); }
  } else {
    el.classList.add('hidden');
    el.style.display = 'none';
  }
};

window.openDialog = window.openModal;
window.closeDialog = window.closeModal;

// Animated Sidebar Toggle Handler
function initSidebarToggle() {
  const isMobile = () => window.innerWidth <= 768;
  const isCollapsed = localStorage.getItem('dawa_sidebar_collapsed') === 'true';
  const sidebar = document.querySelector('.sidebar');
  if (sidebar && isCollapsed && !isMobile()) {
    sidebar.classList.add('collapsed');
  }

  document.addEventListener('click', (e) => {
    const toggleBtn = e.target.closest('[data-sidebar-toggle]');
    if (toggleBtn) {
      e.preventDefault();
      const sb = document.querySelector('.sidebar');
      if (sb) {
        if (isMobile()) {
          sb.classList.toggle('mobile-open');
        } else {
          const collapsed = sb.classList.toggle('collapsed');
          localStorage.setItem('dawa_sidebar_collapsed', collapsed ? 'true' : 'false');
        }
      }
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

  // Auto-activate tab from URL hash if present
  if (window.location.hash) {
    const hash = window.location.hash.substring(1);
    switchTab(hash);
  }
}

function switchTab(tabId, specificList) {
  if (!tabId) return;

  const targetPane = document.getElementById('tab-' + tabId) || document.querySelector(`[data-tab-pane="${tabId}"]`);
  const targetBtn = document.querySelector(`[data-tab-target="${tabId}"]`);

  if (!targetPane) return;

  // Deactivate all sibling panes
  const allPanes = document.querySelectorAll('.tab-pane');
  allPanes.forEach((pane) => pane.classList.remove('active'));

  // Deactivate all tab buttons
  const allBtns = document.querySelectorAll('[data-tab-target]');
  allBtns.forEach((btn) => btn.classList.remove('active'));

  // Activate target
  targetPane.classList.add('active');
  if (targetBtn) targetBtn.classList.add('active');

  // Update hash without scrolling
  history.replaceState(null, null, '#' + tabId);
}

// 2-Step Registration Controller
function initRegistrationStepper() {
  const typeCards = document.querySelectorAll('[data-account-type]');
  const hiddenInput = document.getElementById('reg-account-type-input');
  const badgeLabel = document.getElementById('reg-selected-badge');
  const step1 = document.getElementById('reg-step-1');
  const step2 = document.getElementById('reg-step-2');
  const stepIndicator1 = document.getElementById('step-indicator-1');
  const stepIndicator2 = document.getElementById('step-indicator-2');
  const backBtn = document.getElementById('reg-back-to-step-1');

  if (!step1 || !step2) return;

  typeCards.forEach((card) => {
    card.addEventListener('click', () => {
      const type = card.getAttribute('data-account-type');
      if (hiddenInput) hiddenInput.value = type;

      // Update badge label
      if (badgeLabel) {
        if (type === 'supplier') badgeLabel.textContent = 'نوع الحساب: مورّد / شركة ومستودع أدوية';
        else if (type === 'individual') badgeLabel.textContent = 'نوع الحساب: حساب مهني فردي (صيدلي / باحث عن عمل)';
        else badgeLabel.textContent = 'نوع الحساب: صيدلية / منشأة طبية مرخصة';
      }

      // Conditionally show/hide type specific fields
      document.querySelectorAll('[data-type-visibility]').forEach((el) => {
        const allowed = el.getAttribute('data-type-visibility').split(' ');
        const isVisible = allowed.includes(type);
        el.style.display = isVisible ? 'block' : 'none';
        const input = el.querySelector('input, select');
        if (input && el.hasAttribute('data-was-required')) {
          input.required = isVisible;
        }
      });

      // For individual account, legal name & commercial register are not mandatory
      const legalNameInput = document.getElementById('reg-legal-name');
      const crInput = document.getElementById('reg-cr');
      const legalNameGroup = legalNameInput ? legalNameInput.closest('.form-group') : null;
      const tradeNameGroup = document.getElementById('reg-trade-ar') ? document.getElementById('reg-trade-ar').closest('.grid-group, div') : null;
      const crGroup = crInput ? crInput.closest('.grid-group, div') : null;

      if (type === 'individual') {
        if (legalNameInput) { legalNameInput.required = false; if (legalNameGroup) legalNameGroup.style.display = 'none'; }
        if (crInput) { crInput.required = false; }
        if (tradeNameGroup) { tradeNameGroup.style.display = 'none'; }
        if (crGroup) { crGroup.style.display = 'none'; }
      } else {
        if (legalNameInput) { legalNameInput.required = true; if (legalNameGroup) legalNameGroup.style.display = 'block'; }
        if (crInput) { crInput.required = true; }
        if (tradeNameGroup) { tradeNameGroup.style.display = 'grid'; }
        if (crGroup) { crGroup.style.display = 'grid'; }
      }

      // Switch steps
      step1.classList.remove('active');
      step2.classList.add('active');

      if (stepIndicator1) {
        stepIndicator1.style.color = 'var(--neutral-500)';
        const num = stepIndicator1.querySelector('.step-num');
        if (num) { num.style.background = 'var(--neutral-200)'; num.style.color = 'var(--neutral-700)'; }
      }
      if (stepIndicator2) {
        stepIndicator2.style.color = 'var(--primary-700)';
        const num = stepIndicator2.querySelector('.step-num');
        if (num) { num.style.background = 'var(--primary-600)'; num.style.color = '#fff'; }
      }
    });
  });

  if (backBtn) {
    backBtn.addEventListener('click', () => {
      step2.classList.remove('active');
      step1.classList.add('active');

      if (stepIndicator1) {
        stepIndicator1.style.color = 'var(--primary-700)';
        const num = stepIndicator1.querySelector('.step-num');
        if (num) { num.style.background = 'var(--primary-600)'; num.style.color = '#fff'; }
      }
      if (stepIndicator2) {
        stepIndicator2.style.color = 'var(--neutral-500)';
        const num = stepIndicator2.querySelector('.step-num');
        if (num) { num.style.background = 'var(--neutral-200)'; num.style.color = 'var(--neutral-700)'; }
      }
    });
  }
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
  toast.textContent = message;
  container.appendChild(toast);

  setTimeout(() => {
    toast.classList.add('fade-out');
    setTimeout(() => toast.remove(), 300);
  }, 4000);
}

function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? match[2] : null;
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

