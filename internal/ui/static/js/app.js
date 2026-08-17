// Dawa24 Frontend Application Script
// HTMX + CSRF, flash notices, and request-failure feedback.

document.addEventListener('DOMContentLoaded', () => {
  // Read dawa_csrf cookie and attach X-CSRF-Token to all HTMX requests
  document.body.addEventListener('htmx:configRequest', (evt) => {
    const csrfToken = getCookie('dawa_csrf');
    if (csrfToken) {
      evt.detail.headers['X-CSRF-Token'] = csrfToken;
    }
  });

  // A failed HTMX request used to log to the console and nothing else, so from
  // the reader's side the button simply did nothing. Show it.
  document.body.addEventListener('htmx:responseError', (evt) => {
    const status = evt.detail.xhr.status;
    console.error('Request failed:', status, evt.detail.xhr.responseText);
    showToast(messageForStatus(status), 'error');
  });

  document.body.addEventListener('htmx:sendError', () => {
    showToast('تعذر الاتصال بالخادم. تحقق من اتصالك بالإنترنت.', 'error');
  });

  showNoticeFromQuery();

  // Universal Dropdown Toggle Handler (Native Vanilla JS support)
  document.addEventListener('click', (e) => {
    const dropdownBtn = e.target.closest('.dropdown > button, .dropdown > .btn, [data-dropdown-toggle]');
    const clickedDropdown = dropdownBtn ? dropdownBtn.closest('.dropdown') : null;

    // Close all dropdowns that are not the currently clicked one
    document.querySelectorAll('.dropdown.open').forEach((d) => {
      if (d !== clickedDropdown) {
        d.classList.remove('open');
        const menu = d.querySelector('.dropdown-menu');
        if (menu) menu.classList.remove('is-active');
      }
    });

    if (clickedDropdown) {
      e.stopPropagation();
      const isOpen = clickedDropdown.classList.toggle('open');
      const menu = clickedDropdown.querySelector('.dropdown-menu');
      if (menu) {
        menu.classList.toggle('is-active', isOpen);
      }
    }
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      document.querySelectorAll('.dropdown.open').forEach((d) => {
        d.classList.remove('open');
        const menu = d.querySelector('.dropdown-menu');
        if (menu) menu.classList.remove('is-active');
      });
    }
  });
});

// Renders the ?notice=&msg= pair that redirectWithNotice attaches after a form
// post, then strips it from the URL so a refresh does not repeat the message.
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

// Builds a toast matching the markup in components.css. Kept in JS rather than
// templ because these are raised by client-side events, after render.
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

  // Errors stay until dismissed; a success message is transient.
  if (type !== 'error') {
    setTimeout(() => toast.remove(), 4500);
  }
}

function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}
