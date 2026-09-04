/* ==========================================================================
   DAWA 24 — INTERACTIVE MAP & LEAFLET MODULE (maps.js)
   Coordinate picker, reverse geocoding, radius preview & address locator
   ========================================================================== */

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

/* --------------------------------------------------------------------------
   Standalone helpers.

   registration.js ships richer copies of these for the onboarding flow and,
   when it is on the page, its definitions win (it loads after this file).
   Everywhere else -- branch forms, admin cities, offer locations -- this file
   is the only map code loaded, so it must be able to reverse-geocode and sync
   the governorate dropdowns on its own.
   -------------------------------------------------------------------------- */
if (typeof window.DAWA_EGYPT_CITY_COORDS === 'undefined') {
  window.DAWA_EGYPT_CITY_COORDS = [
    { name: 'القاهرة', lat: 30.0444, lon: 31.2357 },
    { name: 'الجيزة', lat: 30.0131, lon: 31.2089 },
    { name: 'الإسكندرية', lat: 31.2001, lon: 29.9187 },
    { name: 'الدقهلية', lat: 31.0379, lon: 31.3815 },
    { name: 'البحر الأحمر', lat: 27.2579, lon: 33.8116 },
    { name: 'البحيرة', lat: 31.0349, lon: 30.4682 },
    { name: 'الفيوم', lat: 29.3084, lon: 30.8428 },
    { name: 'الغربية', lat: 30.7865, lon: 31.0004 },
    { name: 'الإسماعيلية', lat: 30.5965, lon: 32.2715 },
    { name: 'المنوفية', lat: 30.5972, lon: 30.9876 },
    { name: 'المنيا', lat: 28.1099, lon: 30.7503 },
    { name: 'القليوبية', lat: 30.4591, lon: 31.1786 },
    { name: 'الوادي الجديد', lat: 25.4514, lon: 30.5464 },
    { name: 'السويس', lat: 29.9668, lon: 32.5498 },
    { name: 'أسوان', lat: 24.0889, lon: 32.8998 },
    { name: 'أسيوط', lat: 27.1809, lon: 31.1837 },
    { name: 'بني سويف', lat: 29.0661, lon: 31.0994 },
    { name: 'بورسعيد', lat: 31.2653, lon: 32.3019 },
    { name: 'دمياط', lat: 31.4175, lon: 31.8144 },
    { name: 'الشرقية', lat: 30.5765, lon: 31.5041 },
    { name: 'جنوب سيناء', lat: 28.9712, lon: 33.6176 },
    { name: 'كفر الشيخ', lat: 31.1107, lon: 30.9388 },
    { name: 'مطروح', lat: 31.3543, lon: 27.2373 },
    { name: 'الأقصر', lat: 25.6872, lon: 32.6396 },
    { name: 'قنا', lat: 26.1551, lon: 32.7160 },
    { name: 'شمال سيناء', lat: 31.1316, lon: 33.8033 },
    { name: 'سوهاج', lat: 26.5569, lon: 31.6948 }
  ];
}

if (typeof window.syncCityDropdownsWithCoordinates === 'undefined') {
  window.syncCityDropdownsWithCoordinates = function (lat, lon) {
    let closest = null;
    let minDist = Infinity;
    (window.DAWA_EGYPT_CITY_COORDS || []).forEach(function (c) {
      const d = Math.hypot(lat - c.lat, lon - c.lon);
      if (d < minDist) { minDist = d; closest = c; }
    });

    document.querySelectorAll('[data-city-selector], [data-map-city]').forEach(function (selectEl) {
      if (!selectEl.options) return;
      let bestOpt = null;
      let minOptDist = Infinity;
      for (let i = 0; i < selectEl.options.length; i++) {
        const opt = selectEl.options[i];
        if (!opt.value) continue;
        let oLat = parseFloat(opt.dataset.lat);
        let oLon = parseFloat(opt.dataset.lng || opt.dataset.lon);
        if (isNaN(oLat) || isNaN(oLon)) {
          const parts = String(opt.value).split(',').map(function (v) { return parseFloat(v.trim()); });
          oLat = parts[0]; oLon = parts[1];
        }
        if (!isNaN(oLat) && !isNaN(oLon)) {
          const d = Math.hypot(lat - oLat, lon - oLon);
          if (d < minOptDist) { minOptDist = d; bestOpt = opt; }
        }
      }
      if (bestOpt && minOptDist < 0.45) {
        selectEl.value = bestOpt.value;
        const cityId = bestOpt.dataset.cityId;
        if (cityId) {
          document.querySelectorAll('[data-map-city-id], input[name="branch_city_id"], input[name="city_id"]').forEach(function (hi) {
            hi.value = cityId;
          });
        }
      }
    });

    return closest;
  };
}

if (typeof window.fetchDetailedAddressFromCoords === 'undefined') {
  let reverseGeocodeTimer = null;
  window.fetchDetailedAddressFromCoords = function (lat, lon) {
    const addressInput = document.getElementById('reg-address') ||
      document.querySelector('input[name="address"], [data-map-address]');
    if (!addressInput) return;
    clearTimeout(reverseGeocodeTimer);
    reverseGeocodeTimer = setTimeout(async function () {
      try {
        const resp = await fetch(
          'https://nominatim.openstreetmap.org/reverse?format=json&lat=' + lat +
          '&lon=' + lon + '&zoom=18&addressdetails=1&accept-language=ar',
          { headers: { 'Accept': 'application/json' } }
        );
        if (!resp.ok) return;
        const data = await resp.json();
        if (!data || !data.address) return;
        const a = data.address;
        const parts = [];
        if (a.road || a.street) parts.push(a.road || a.street);
        if (a.neighbourhood || a.suburb || a.quarter) parts.push(a.neighbourhood || a.suburb || a.quarter);
        if (a.city || a.town || a.village || a.county) parts.push(a.city || a.town || a.village || a.county);
        if (a.state) parts.push(a.state);
        const fullAddr = parts.filter(Boolean).join('، ');
        if (fullAddr && !addressInput.value.trim()) addressInput.value = fullAddr;
      } catch (e) {
        console.warn('reverse geocoding:', e);
      }
    }, 400);
  };
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
    // The "my location" button is bound once, globally, at the foot of this
    // file. It used to be resolved here — a single element per map, looked up
    // when the map initialised — which meant a button rendered later (inside a
    // modal, an Alpine template) never got a listener, and a page with four
    // pickers could bind the first button to the wrong map.

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
      html: `<div style="width:36px; height:36px; display:flex; align-items:center; justify-content:center; background:#0ea5e9; color:#fff; border-radius:50%; box-shadow:0 4px 14px rgba(14,165,233,0.5); border:3px solid #ffffff; font-size:18px; cursor:grab; transform:translate(-50%, -50%);">ðŸ“</div>`,
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
        latInput.dispatchEvent(new Event('change', { bubbles: true }));
      }
      if (lonInput) {
        lonInput.value = fixedLon.toFixed(6);
        lonInput.dispatchEvent(new Event('input', { bubbles: true }));
        lonInput.dispatchEvent(new Event('change', { bubbles: true }));
      }

      window.dispatchEvent(new CustomEvent('dawa-coords-change', {
        detail: {
          lat: fixedLat,
          lon: fixedLon,
          targetId: container.id || '',
          container: container
        }
      }));

      const gmapsUrl = `https://www.google.com/maps?q=${fixedLat},${fixedLon}`;
      if (gmapsInput) gmapsInput.value = gmapsUrl;
      gmapsLinks.forEach((link) => { link.href = gmapsUrl; });
      if (badge) badge.textContent = `${fixedLat.toFixed(4)}, ${fixedLon.toFixed(4)}`;

      // Sync city selectors and hidden city ID
      if (typeof window.syncCityDropdownsWithCoordinates === 'function') {
        window.syncCityDropdownsWithCoordinates(fixedLat, fixedLon);
      }

      // Auto-fetch detailed street/district address via Reverse Geocoding
      if (typeof window.fetchDetailedAddressFromCoords === 'function') {
        window.fetchDetailedAddressFromCoords(fixedLat, fixedLon);
      }
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
            showToast('تم تعيين المنطقة تلقائياً: ' + name + ' 📍', 'info');
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
            showToast('تم استخراج وتعيين الإحداثيات تلقائياً من الرابط 📍', 'success');
          }
        }
      };
      gmapsInput.addEventListener('input', onGmapsUrlInput);
      gmapsInput.addEventListener('paste', () => setTimeout(onGmapsUrlInput, 50));
    }

    // Auto Invalidate Size for Modals and Tabs with Debounce
    const modalParent = container.closest('dialog, .modal, .tab-pane');
    if (modalParent) {
      let isInvalidating = false;
      const resizeObserver = new MutationObserver(() => {
        if (isInvalidating) return;
        const isOpen = modalParent.hasAttribute('open') || modalParent.classList.contains('active') || modalParent.offsetParent !== null;
        if (isOpen) {
          isInvalidating = true;
          requestAnimationFrame(() => {
            map.invalidateSize();
            setTimeout(() => {
              map.invalidateSize();
              isInvalidating = false;
            }, 250);
          });
        }
      });
      resizeObserver.observe(modalParent, { attributes: true, attributeFilter: ['style', 'class', 'open'] });
    }

    let resizeTimer = null;
    window.addEventListener('resize', () => {
      if (resizeTimer) return;
      resizeTimer = requestAnimationFrame(() => {
        map.invalidateSize();
        resizeTimer = null;
      });
    }, { passive: true });

    setTimeout(() => map.invalidateSize(), 150);
    setTimeout(() => map.invalidateSize(), 400);
  });
}


if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => {
    initMapPickers();
  });
} else {
  initMapPickers();
}

/**
 * Programmatically set the location of a map picker and pan/zoom its Leaflet instance.
 * Used by branch managers and admin city modals on edit.
 */
window.dawaSetMapLocation = function(target, lat, lon, zoom) {
  var el = typeof target === 'string' ? document.querySelector(target) : target;
  if (!el) return;
  var pLat = parseFloat(lat);
  var pLon = parseFloat(lon);
  if (isNaN(pLat) || isNaN(pLon)) return;

  var z = (zoom !== undefined && zoom !== null) ? zoom : 14;

  function triggerInvalidate(mapInstance) {
    if (!mapInstance) return;
    mapInstance.invalidateSize();
    setTimeout(function() { mapInstance.invalidateSize(); }, 60);
    setTimeout(function() { mapInstance.invalidateSize(); }, 200);
    setTimeout(function() { mapInstance.invalidateSize(); }, 400);
  }

  function doUpdate(container) {
    if (typeof container._updateCoords === 'function') {
      container._updateCoords(pLat, pLon, z);
      if (container._leaflet_map) {
        triggerInvalidate(container._leaflet_map);
      }
      return true;
    }
    var canvas = container.querySelector ? container.querySelector('.map-canvas, .map-container, [data-map-canvas], .leaflet-map-canvas') : null;
    if (canvas && typeof canvas._updateCoords === 'function') {
      canvas._updateCoords(pLat, pLon, z);
      if (canvas._leaflet_map) {
        triggerInvalidate(canvas._leaflet_map);
      }
      return true;
    }
    return false;
  }

  if (!doUpdate(el)) {
    el.dataset.defaultLat = pLat;
    el.dataset.defaultLon = pLon;
    initMapPickers();
    setTimeout(function() { doUpdate(el); }, 150);
  }
};
window.setMapPickerLocation = window.dawaSetMapLocation;

/* --------------------------------------------------------------------------
   "موقعي الحالي" — one delegated handler for the eight buttons that ask for it.

   The markup is duplicated across map_picker.templ, admin_cities.templ (four
   of them), customer_branches.templ, customer_branch_form.templ,
   vendor_branches.templ, vendor_branch_form.templ and
   vendor_offer_locations.templ. The behaviour never was: there is one
   implementation, and it used to be bound per map at initialisation. Three
   things followed, and all three read to a user as "the button does nothing":

     - A button rendered after initMapPickers ran — inside a modal, an Alpine
       template, an htmx swap — never got a listener at all.
     - A page with several pickers resolved the button through a document-wide
       fallback, so the first button could drive the wrong map.
     - The failure messages did not distinguish "your browser will not do this
       over plain HTTP" from "you denied permission" from "the fix timed out",
       and the label was overwritten with a hard-coded string that then replaced
       whatever that particular button said.

   Delegation fixes the first two: the listener is on the document, so it does
   not care when the button appeared.
   -------------------------------------------------------------------------- */
(function () {
  'use strict';

  var LOCATE_SELECTOR = '[data-map-locate], [data-locate-me-btn], .btn-locate';

  function toast(message, kind) {
    if (typeof window.showToast === 'function') {
      window.showToast(message, kind || 'info');
    } else {
      console.warn('[locate]', message);
    }
  }

  function setBusy(btn, busy) {
    if (busy) {
      if (btn.dataset.originalLabel === undefined) {
        btn.dataset.originalLabel = btn.innerHTML;
      }
      btn.disabled = true;
      btn.setAttribute('aria-busy', 'true');
      btn.textContent = 'جارٍ التحديد…';
      return;
    }
    btn.disabled = false;
    btn.removeAttribute('aria-busy');
    if (btn.dataset.originalLabel !== undefined) {
      // Restore this button's own wording. Overwriting it with one hard-coded
      // string made every locate button on the page say the same thing after
      // the first click.
      btn.innerHTML = btn.dataset.originalLabel;
    }
  }

  function applyPosition(btn, pos) {
    var container = btn.closest('[data-map-picker]');
    var setCoords = container && container._updateCoords;
    var lat = pos.coords.latitude;
    var lon = pos.coords.longitude;

    if (typeof setCoords === 'function') {
      setCoords(lat, lon, 16);
    } else {
      // No map beside this button: still fill whatever coordinate inputs the
      // surrounding form has, so the button is not simply inert.
      var scope = btn.closest('form') || document;
      var latInput = scope.querySelector('[data-map-input="lat"], input[name="latitude"]');
      var lonInput = scope.querySelector('[data-map-input="lon"], input[name="longitude"]');
      if (latInput) { latInput.value = lat.toFixed(8); latInput.dispatchEvent(new Event('input', { bubbles: true })); }
      if (lonInput) { lonInput.value = lon.toFixed(8); lonInput.dispatchEvent(new Event('input', { bubbles: true })); }
    }

    var nearest = (typeof window.syncCityDropdownsWithCoordinates === 'function')
      ? window.syncCityDropdownsWithCoordinates(lat, lon)
      : null;
    if (nearest && nearest.name) {
      toast('تم تحديد موقعك وتحديث المنطقة إلى: ' + nearest.name, 'success');
    } else {
      toast('تم تحديد موقعك الجغرافي.', 'success');
    }
  }

  function describeError(err) {
    if (!err) return 'تعذّر تحديد موقعك. حاول مرة أخرى.';
    switch (err.code) {
      case 1: // PERMISSION_DENIED
        return 'تم رفض إذن الوصول للموقع. فعّل إذن الموقع لهذا الموقع من إعدادات المتصفح ثم حاول مجدداً.';
      case 2: // POSITION_UNAVAILABLE
        return 'خدمة تحديد الموقع غير متاحة على هذا الجهاز حالياً. تأكد من تفعيل GPS أو خدمات الموقع.';
      case 3: // TIMEOUT
        return 'استغرق تحديد الموقع وقتاً أطول من المتوقع. حاول مرة أخرى في مكان مكشوف.';
      default:
        return 'تعذّر تحديد موقعك. حاول مرة أخرى.';
    }
  }

  /* Two stages, deliberately.
   *
   * A single high-accuracy request with maximumAge: 0 is what the previous
   * version sent, and on Safari — macOS and iOS both — a cold receiver
   * routinely spends longer than its ten-second timeout getting a first fix,
   * so the button reported failure on a device that would have answered. A
   * coarse fix arrives in about a second and is accurate enough to place a
   * pharmacy branch; the precise one then refines it if it arrives. */
  function locate(btn) {
    if (!window.isSecureContext) {
      toast('تحديد الموقع يتطلب اتصالاً آمناً (HTTPS). افتح الموقع عبر رابط آمن ثم حاول مجدداً.', 'warning');
      return;
    }
    if (!navigator.geolocation) {
      toast('متصفحك لا يدعم خاصية تحديد الموقع. أدخل الإحداثيات يدوياً أو الصق رابط خرائط Google.', 'warning');
      return;
    }

    setBusy(btn, true);
    var settled = false;

    navigator.geolocation.getCurrentPosition(
      function (pos) {
        settled = true;
        setBusy(btn, false);
        applyPosition(btn, pos);
        // Refine quietly. A failure here is not a failure of the button: the
        // coarse fix is already applied and reported.
        navigator.geolocation.getCurrentPosition(
          function (precise) { applyPosition(btn, precise); },
          function () {},
          { enableHighAccuracy: true, timeout: 15000, maximumAge: 0 }
        );
      },
      function (err) {
        if (settled) return;
        setBusy(btn, false);
        console.warn('geolocation:', err && err.message);
        toast(describeError(err), 'warning');
      },
      { enableHighAccuracy: false, timeout: 8000, maximumAge: 60000 }
    );
  }

  document.addEventListener('click', function (e) {
    var btn = e.target && e.target.closest ? e.target.closest(LOCATE_SELECTOR) : null;
    if (!btn || btn.disabled) return;
    e.preventDefault();
    locate(btn);
  });
})();
