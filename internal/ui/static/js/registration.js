/* ==========================================================================
   DAWA 24 — REGISTRATION & ONBOARDING MODULE (registration.js)
   Multi-step stepper, file upload preview, verification & password strength
   ========================================================================== */

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
          num.textContent = 'âœ“';
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
        if (type === 'supplier' || type === 'vendor') badgeLabel.textContent = 'Ù†ÙˆØ¹ Ø§Ù„Ø­Ø³Ø§Ø¨: Ù…ÙˆØ±Ù‘Ø¯ / Ø´Ø±ÙƒØ© ÙˆÙ…Ø³ØªÙˆØ¯Ø¹ Ø£Ø¯ÙˆÙŠØ©';
        else if (type === 'job_seeker') badgeLabel.textContent = 'Ù†ÙˆØ¹ Ø§Ù„Ø­Ø³Ø§Ø¨: Ø¨Ø§Ø­Ø« Ø¹Ù† Ø¹Ù…Ù„ / ÙƒØ§Ø¯Ø± Ø·Ø¨ÙŠ ÙˆØµÙŠØ¯Ù„Ø§Ù†ÙŠ';
        else badgeLabel.textContent = 'Ù†ÙˆØ¹ Ø§Ù„Ø­Ø³Ø§Ø¨: ØµÙŠØ¯Ù„ÙŠØ© / Ù…Ù†Ø´Ø£Ø© Ø·Ø¨ÙŠØ© Ù…Ø±Ø®ØµØ©';
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
        showToast('ÙŠØ±Ø¬Ù‰ ÙƒØªØ§Ø¨Ø© Ø§Ù„Ø§Ø³Ù… Ø§Ù„Ù‚Ø§Ù†ÙˆÙ†ÙŠ Ù„Ù„Ù…Ù†Ø´Ø£Ø©.', 'warning');
        return;
      }
      if (tradeAr && tradeAr.required && !tradeAr.value.trim()) {
        tradeAr.focus();
        showToast('ÙŠØ±Ø¬Ù‰ ÙƒØªØ§Ø¨Ø© Ø§Ù„Ø§Ø³Ù… Ø§Ù„ØªØ¬Ø§Ø±ÙŠ Ù„Ù„Ù…Ù†Ø´Ø£Ø©.', 'warning');
        return;
      }
      if (cr && cr.required && !cr.value.trim()) {
        cr.focus();
        showToast('ÙŠØ±Ø¬Ù‰ ÙƒØªØ§Ø¨Ø© Ø±Ù‚Ù… Ø§Ù„Ø³Ø¬Ù„ Ø§Ù„ØªØ¬Ø§Ø±ÙŠ.', 'warning');
        return;
      }
      if (address && address.required && !address.value.trim()) {
        address.focus();
        showToast('ÙŠØ±Ø¬Ù‰ ØªØ­Ø¯ÙŠØ¯ Ù…ÙˆÙ‚Ø¹ Ø§Ù„Ù…Ù†Ø´Ø£Ø© ÙˆÙƒØªØ§Ø¨Ø© Ø§Ù„Ø¹Ù†ÙˆØ§Ù† Ø§Ù„ØªÙØµÙŠÙ„ÙŠ.', 'warning');
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
      togglePwdBtn.textContent = isPass ? 'ðŸ”’' : 'ðŸ‘ï¸';
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
          if (icon) { icon.textContent = 'âœ“'; icon.style.color = 'var(--success, #10b981)'; }
        } else {
          el.style.color = 'var(--text-secondary)';
          if (icon) { icon.textContent = 'â—'; icon.style.color = 'var(--text-muted)'; }
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
      const labels = ['Ø£Ø¯Ø®Ù„ ÙƒÙ„Ù…Ø© Ø§Ù„Ù…Ø±ÙˆØ±', 'Ø¶Ø¹ÙŠÙØ© Ø¬Ø¯Ø§Ù‹ ðŸ”´', 'Ø¶Ø¹ÙŠÙØ© ðŸŸ ', 'Ù…ØªÙˆØ³Ø·Ø© ðŸŸ¡', 'Ù‚ÙˆÙŠØ© ÙˆÙ…Ø·Ø§Ø¨Ù‚Ø© Ù„Ù„Ù…Ø¹Ø§ÙŠÙŠØ± ðŸŸ¢'];

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
        showToast('ÙŠØ±Ø¬Ù‰ Ø¥Ø¯Ø®Ø§Ù„ Ø§Ø³Ù… Ø§Ù„Ù…Ø³Ø¤ÙˆÙ„ / Ø§Ù„ØµÙŠØ¯Ù„ÙŠ Ø§Ù„Ù…Ø³Ø¤ÙˆÙ„.', 'warning');
        return;
      }
      if (email && !email.value.trim()) {
        e.preventDefault();
        email.focus();
        showToast('ÙŠØ±Ø¬Ù‰ Ø¥Ø¯Ø®Ø§Ù„ Ø§Ù„Ø¨Ø±ÙŠØ¯ Ø§Ù„Ø¥Ù„ÙƒØªØ±ÙˆÙ†ÙŠ Ø§Ù„Ø±Ø³Ù…ÙŠ.', 'warning');
        return;
      }
      if (phone && !phone.value.trim()) {
        e.preventDefault();
        phone.focus();
        showToast('ÙŠØ±Ø¬Ù‰ Ø¥Ø¯Ø®Ø§Ù„ Ø±Ù‚Ù… Ø§Ù„Ù‡Ø§ØªÙ.', 'warning');
        return;
      }
      if (password) {
        const val = password.value;
        if (val.length < 8) {
          e.preventDefault();
          password.focus();
          showToast('ÙƒÙ„Ù…Ø© Ø§Ù„Ù…Ø±ÙˆØ± ÙŠØ¬Ø¨ Ø£Ù† Ù„Ø§ ØªÙ‚Ù„ Ø¹Ù† 8 Ø£Ø­Ø±Ù.', 'warning');
          return;
        }
        const hasUpper = /[A-Z]/.test(val);
        const hasLower = /[a-z]/.test(val);
        const hasNum = /[0-9]/.test(val);
        const hasSpec = /[^A-Za-z0-9]/.test(val);
        if (!hasUpper || !hasLower || !hasNum || !hasSpec) {
          e.preventDefault();
          password.focus();
          showToast('ÙƒÙ„Ù…Ø© Ø§Ù„Ù…Ø±ÙˆØ± ÙŠØ¬Ø¨ Ø£Ù† ØªØ­ØªÙˆÙŠ Ø¹Ù„Ù‰ Ø£Ø­Ø±Ù ÙƒØ¨ÙŠØ±Ø© ÙˆØµØºÙŠØ±Ø© ÙˆØ£Ø±Ù‚Ø§Ù… ÙˆØ±Ù…ÙˆØ² Ø®Ø§ØµØ© (@, $, !).', 'warning');
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

      if (hint) hint.textContent = 'â³ Ø¬Ù„Ø¨ Ø§Ù„Ø¹Ù†ÙˆØ§Ù† Ø¨Ø§Ù„ØªÙØµÙŠÙ„...';
      const resp = await fetch(`https://nominatim.openstreetmap.org/reverse?format=json&lat=${lat}&lon=${lon}&zoom=18&addressdetails=1&accept-language=ar`, {
        headers: { 'Accept': 'application/json' }
      });
      if (!resp.ok) {
        if (hint) hint.textContent = 'ðŸ“ ØªÙ… ØªØ­Ø¯ÙŠØ¯ Ø§Ù„Ø¥Ø­Ø¯Ø§Ø«ÙŠØ§Øª Ø¨Ù†Ø¬Ø§Ø­';
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

        const fullAddr = parts.filter(Boolean).join('ØŒ ');
        if (fullAddr) {
          addressInput.value = fullAddr;
          if (hint) hint.textContent = 'ðŸ“ ØªÙ… ØªØ­Ø¯ÙŠØ« Ø§Ù„Ø¹Ù†ÙˆØ§Ù† ØªÙ„Ù‚Ø§Ø¦ÙŠØ§Ù‹ Ù…Ù† Ø§Ù„Ø®Ø±ÙŠØ·Ø©';
        }
      }
    } catch (e) {
      console.warn('Reverse geocoding error:', e);
    }
  }, 400);
}

// Egyptian Cities Coordinates Reference Table
const EGYPT_CITIES_COORDS = [
  { name: 'Ø§Ù„Ù‚Ø§Ù‡Ø±Ø©', lat: 30.0444, lon: 31.2357 },
  { name: 'Ø§Ù„Ù‚Ø§Ù‡Ø±Ø© Ø§Ù„Ø¬Ø¯ÙŠØ¯Ø©', lat: 30.0131, lon: 31.2089 },
  { name: 'Ø§Ù„Ø´Ø±ÙˆÙ‚', lat: 30.1219, lon: 31.3665 },
  { name: 'Ù…Ø¯ÙŠÙ†Ø© Ø¨Ø¯Ø±', lat: 30.1842, lon: 31.2482 },
  { name: 'Ø§Ù„Ø¬ÙŠØ²Ø©', lat: 30.0131, lon: 31.2089 },
  { name: 'Ù…Ø¯ÙŠÙ†Ø© Ø³ØªØ© Ø£ÙƒØªÙˆØ¨Ø±', lat: 30.0648, lon: 30.9706 },
  { name: 'Ø§Ù„Ø´ÙŠØ® Ø²Ø§ÙŠØ¯', lat: 30.1111, lon: 30.8544 },
  { name: 'Ø§Ù„Ø­ÙˆØ§Ù…Ø¯ÙŠØ©', lat: 29.9667, lon: 31.3000 },
  { name: 'Ø£ÙˆØ³ÙŠÙ…', lat: 29.8833, lon: 31.2333 },
  { name: 'Ø§Ù„Ø¨Ø¯Ø±Ø´ÙŠÙ†', lat: 29.8167, lon: 31.2833 },
  { name: 'Ø§Ù„Ø¥Ø³ÙƒÙ†Ø¯Ø±ÙŠØ©', lat: 31.2001, lon: 29.9187 },
  { name: 'Ø¨Ø±Ø¬ Ø§Ù„Ø¹Ø±Ø¨', lat: 31.0333, lon: 29.7667 },
  { name: 'Ù…Ø¯ÙŠÙ†Ø© Ø¨Ø±Ø¬ Ø§Ù„Ø¹Ø±Ø¨ Ø§Ù„Ø¬Ø¯ÙŠØ¯Ø©', lat: 30.9164, lon: 29.5553 },
  { name: 'Ø´Ø¨Ø±Ø§ Ø§Ù„Ø®ÙŠÙ…Ø©', lat: 30.4500, lon: 31.1833 },
  { name: 'Ø§Ù„Ø®ØµÙˆØµ', lat: 30.4667, lon: 31.1833 },
  { name: 'Ø¨Ù†Ù‡Ø§', lat: 30.4667, lon: 31.1833 },
  { name: 'Ù‚Ù„ÙŠÙˆØ¨', lat: 30.1833, lon: 31.2167 },
  { name: 'Ø§Ù„Ø¹Ø¨ÙˆØ±', lat: 30.2000, lon: 31.3167 },
  { name: 'Ø¨ÙˆØ± Ø³Ø¹ÙŠØ¯', lat: 31.2654, lon: 32.3020 },
  { name: 'Ø¨ÙˆØ± ÙØ¤Ø§Ø¯', lat: 31.2333, lon: 32.3167 },
  { name: 'Ø§Ù„Ø³ÙˆÙŠØ³', lat: 29.9668, lon: 32.5498 },
  { name: 'Ø§Ù„Ø¥Ø³Ù…Ø§Ø¹ÙŠÙ„ÙŠØ©', lat: 30.5965, lon: 32.2715 },
  { name: 'ÙØ§ÙŠØ¯', lat: 30.3333, lon: 32.3000 },
  { name: 'Ø§Ù„Ù‚Ù†Ø·Ø±Ø© Ø´Ø±Ù‚', lat: 30.8333, lon: 32.3167 },
  { name: 'Ø·Ù†Ø·Ø§', lat: 30.7865, lon: 31.0004 },
  { name: 'Ø§Ù„Ù…Ø­Ù„Ø© Ø§Ù„ÙƒØ¨Ø±Ù‰', lat: 30.9667, lon: 31.1667 },
  { name: 'ÙƒÙØ± Ø§Ù„Ø²ÙŠØ§Øª', lat: 30.8167, lon: 30.8167 },
  { name: 'Ø²ÙØªÙ‰', lat: 30.7167, lon: 31.2500 },
  { name: 'Ø§Ù„Ù…Ù†ØµÙˆØ±Ø©', lat: 31.0409, lon: 31.3785 },
  { name: 'Ù…ÙŠØª ØºÙ…Ø±', lat: 30.7167, lon: 31.2500 },
  { name: 'Ø§Ù„Ø³Ù†Ø¨Ù„Ø§ÙˆÙŠÙ†', lat: 30.8833, lon: 31.4667 },
  { name: 'Ø¯ÙƒØ±Ù†Ø³', lat: 31.0833, lon: 31.6000 },
  { name: 'Ø¨Ù„Ù‚Ø§Ø³', lat: 31.2333, lon: 31.3667 },
  { name: 'Ø·Ù„Ø®Ø§', lat: 31.0500, lon: 31.3667 },
  { name: 'Ø§Ù„Ø²Ù‚Ø§Ø²ÙŠÙ‚', lat: 30.5877, lon: 31.5020 },
  { name: 'Ø§Ù„Ø¹Ø§Ø´Ø± Ù…Ù† Ø±Ù…Ø¶Ø§Ù†', lat: 30.3000, lon: 31.7333 },
  { name: 'Ø¨Ù„Ø¨ÙŠØ³', lat: 30.4167, lon: 31.5667 },
  { name: 'ÙØ§Ù‚ÙˆØ³', lat: 30.7333, lon: 31.8000 },
  { name: 'Ù…Ù†ÙŠØ§ Ø§Ù„Ù‚Ù…Ø­', lat: 30.5167, lon: 31.3500 },
  { name: 'Ø£Ø¨Ùˆ Ø­Ù…Ø§Ø¯', lat: 30.5500, lon: 31.6833 },
  { name: 'Ø£Ø¨Ùˆ ÙƒØ¨ÙŠØ±', lat: 30.7333, lon: 31.6667 },
  { name: 'Ø¯Ù…Ù†Ù‡ÙˆØ±', lat: 31.0409, lon: 30.4667 },
  { name: 'ÙƒÙØ± Ø§Ù„Ø¯ÙˆØ§Ø±', lat: 31.1333, lon: 30.1333 },
  { name: 'Ø¥Ø¯ÙƒÙˆ', lat: 31.3000, lon: 30.3000 },
  { name: 'Ø±Ø´ÙŠØ¯', lat: 31.4000, lon: 30.4167 },
  { name: 'Ø´Ø¨ÙŠÙ† Ø§Ù„ÙƒÙˆÙ…', lat: 30.5522, lon: 31.0094 },
  { name: 'Ù…Ù†ÙˆÙ', lat: 30.4667, lon: 30.9333 },
  { name: 'Ø£Ø´Ù…ÙˆÙ†', lat: 30.3000, lon: 30.9833 },
  { name: 'Ù‚ÙˆÙŠØ³Ù†Ø§', lat: 30.5667, lon: 31.1500 },
  { name: 'Ù…Ø¯ÙŠÙ†Ø© Ø§Ù„Ø³Ø§Ø¯Ø§Øª', lat: 30.3833, lon: 30.5167 },
  { name: 'ÙƒÙØ± Ø§Ù„Ø´ÙŠØ®', lat: 31.1107, lon: 30.9388 },
  { name: 'Ø¯Ø³ÙˆÙ‚', lat: 31.1333, lon: 30.6500 },
  { name: 'ÙÙˆÙ‡', lat: 31.2000, lon: 30.5500 },
  { name: 'Ø¯Ù…ÙŠØ§Ø·', lat: 31.4165, lon: 31.8133 },
  { name: 'Ø¯Ù…ÙŠØ§Ø· Ø§Ù„Ø¬Ø¯ÙŠØ¯Ø©', lat: 31.4333, lon: 31.6667 },
  { name: 'Ø±Ø£Ø³ Ø§Ù„Ø¨Ø±', lat: 31.5167, lon: 31.8167 },
  { name: 'Ø§Ù„ÙÙŠÙˆÙ…', lat: 29.3084, lon: 30.8428 },
  { name: 'Ø¨Ù†ÙŠ Ø³ÙˆÙŠÙ', lat: 29.0661, lon: 31.0994 },
  { name: 'Ø§Ù„Ù…Ù†ÙŠØ§', lat: 28.1099, lon: 30.7503 },
  { name: 'Ù…Ù„ÙˆÙŠ', lat: 27.7333, lon: 30.8333 },
  { name: 'Ø£Ø³ÙŠÙˆØ·', lat: 27.1801, lon: 31.1837 },
  { name: 'Ø¯ÙŠØ±ÙˆØ·', lat: 27.5667, lon: 30.8167 },
  { name: 'Ø³ÙˆÙ‡Ø§Ø¬', lat: 26.5569, lon: 31.6948 },
  { name: 'Ø·Ù‡Ø·Ø§', lat: 26.7667, lon: 31.5000 },
  { name: 'Ø¬Ø±Ø¬Ø§', lat: 26.3333, lon: 31.8833 },
  { name: 'Ù‚Ù†Ø§', lat: 26.1642, lon: 32.7267 },
  { name: 'Ù†Ø¬Ø¹ Ø­Ù…Ø§Ø¯ÙŠ', lat: 26.0500, lon: 32.2500 },
  { name: 'Ø§Ù„Ø£Ù‚ØµØ±', lat: 25.6872, lon: 32.6396 },
  { name: 'Ø¥Ø³Ù†Ø§', lat: 25.2833, lon: 32.5500 },
  { name: 'Ø£Ø³ÙˆØ§Ù†', lat: 24.0889, lon: 32.8998 },
  { name: 'Ø¥Ø¯ÙÙˆ', lat: 24.9833, lon: 32.8667 },
  { name: 'ÙƒÙˆÙ… Ø£Ù…Ø¨Ùˆ', lat: 24.4667, lon: 32.9500 },
  { name: 'Ù…Ø·Ø±ÙˆØ­', lat: 31.3525, lon: 27.2453 },
  { name: 'Ø§Ù„Ø¹Ù„Ù…ÙŠÙ†', lat: 30.8333, lon: 28.9500 },
  { name: 'Ø³ÙŠÙˆØ©', lat: 29.2000, lon: 25.5167 },
  { name: 'Ø§Ù„ØºØ±Ø¯Ù‚Ø©', lat: 27.2579, lon: 33.8116 },
  { name: 'Ø³ÙØ§Ø¬Ø§', lat: 26.7292, lon: 33.9365 },
  { name: 'Ù…Ø±Ø³Ù‰ Ø¹Ù„Ù…', lat: 25.0676, lon: 34.8966 },
  { name: 'Ø§Ù„Ø®Ø§Ø±Ø¬Ø©', lat: 25.4390, lon: 30.5586 },
  { name: 'Ø§Ù„Ø¯Ø§Ø®Ù„Ø©', lat: 25.5167, lon: 28.9667 },
  { name: 'Ø§Ù„Ø¹Ø±ÙŠØ´', lat: 31.1316, lon: 33.7984 },
  { name: 'Ø§Ù„Ø·ÙˆØ±', lat: 28.2410, lon: 33.6230 },
  { name: 'Ø´Ø±Ù… Ø§Ù„Ø´ÙŠØ®', lat: 27.9158, lon: 34.3299 },
  { name: 'Ø¯Ù‡Ø¨', lat: 28.5094, lon: 34.5137 },
  { name: 'Ù†ÙˆÙŠØ¨Ø¹', lat: 29.0436, lon: 34.6644 },
  { name: 'Ø·Ø§Ø¨Ø§', lat: 29.4925, lon: 34.8967 },
  { name: 'Ø±Ø£Ø³ Ø³Ø¯Ø±', lat: 29.5892, lon: 32.7144 }
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


if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => {
    initRegistrationStepper();
  });
} else {
  initRegistrationStepper();
}
