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

  // 6. Universal Dropdown Toggle Handler
  initDropdowns();

  // 7. Animated Sidebar Toggle
  initSidebarToggle();
});

// Animated Sidebar Toggle Handler
function initSidebarToggle() {
  const isCollapsed = localStorage.getItem('dawa_sidebar_collapsed') === 'true';
  const sidebar = document.querySelector('.sidebar');
  if (sidebar && isCollapsed) {
    sidebar.classList.add('collapsed');
  }

  document.addEventListener('click', (e) => {
    const toggleBtn = e.target.closest('[data-sidebar-toggle]');
    if (toggleBtn) {
      const sb = document.querySelector('.sidebar');
      if (sb) {
        const collapsed = sb.classList.toggle('collapsed');
        localStorage.setItem('dawa_sidebar_collapsed', collapsed ? 'true' : 'false');
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
        if (type === 'supplier') badgeLabel.textContent = 'نوع الحساب: مورّد أدوية ومستودع';
        else if (type === 'chain_pharmacy') badgeLabel.textContent = 'نوع الحساب: مجموعة وسلسلة صيدليات';
        else badgeLabel.textContent = 'نوع الحساب: صيدلية مرخصة';
      }

      // Conditionally show/hide type specific fields
      document.querySelectorAll('[data-type-visibility]').forEach((el) => {
        const allowed = el.getAttribute('data-type-visibility').split(' ');
        el.style.display = allowed.includes(type) ? 'block' : 'none';
      });

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

// Modal Open/Close Manager
function initModalManager() {
  document.addEventListener('click', (e) => {
    const openBtn = e.target.closest('[data-modal-open]');
    if (openBtn) {
      const modalId = openBtn.getAttribute('data-modal-open');
      const modal = document.getElementById(modalId);
      if (modal) {
        modal.classList.remove('hidden');
      }
    }

    const closeBtn = e.target.closest('[data-modal-close]');
    if (closeBtn) {
      const modal = closeBtn.closest('.modal-overlay');
      if (modal) {
        modal.classList.add('hidden');
      }
    }

    // Click outside modal card to close
    if (e.target.classList.contains('modal-overlay')) {
      e.target.classList.add('hidden');
    }
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      document.querySelectorAll('.modal-overlay').forEach((m) => m.classList.add('hidden'));
    }
  });
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
    case 409: return 'تم تعديل هذا السجل من مكان آخر. أعد تحميل الصفحة.';
    case 422: return 'البيانات المدخلة غير صالحة.';
    default:  return 'تعذر إتمام العملية. يرجى المحاولة مرة أخرى.';
  }
}

function showToast(message, type) {
  let stack = document.querySelector('.toast-stack');
  if (!stack) {
    stack = document.createElement('div');
    stack.className = 'toast-stack';
    stack.setAttribute('aria-live', 'polite');
    stack.setAttribute('aria-atomic', 'true');
    document.body.appendChild(stack);
  }

  const toast = document.createElement('div');
  toast.className = 'toast toast-' + (type || 'info');
  toast.setAttribute('role', 'status');

  const text = document.createElement('div');
  text.style.flex = '1';
  text.textContent = message;

  const close = document.createElement('button');
  close.className = 'toast-close';
  close.setAttribute('aria-label', 'إغلاق');
  close.textContent = '✕';
  close.addEventListener('click', () => toast.remove());

  toast.appendChild(text);
  toast.appendChild(close);
  stack.appendChild(toast);

  if (type !== 'error') {
    setTimeout(() => toast.remove(), 4500);
  }
}

function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}
