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
            showToast(`ØªÙ… ØªØ­Ø±ÙŠÙƒ Ø§Ù„Ø®Ø±ÙŠØ·Ø© ØªÙ„Ù‚Ø§Ø¦ÙŠØ§Ù‹ Ø¥Ù„Ù‰: ${name} ðŸ“`, 'info');
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
            showToast('ØªÙ… Ø§Ø³ØªØ®Ø±Ø§Ø¬ ÙˆØªØ­Ø¯ÙŠØ« Ø§Ù„Ø¥Ø­Ø¯Ø§Ø«ÙŠØ§Øª ØªÙ„Ù‚Ø§Ø¦ÙŠØ§Ù‹ Ù…Ù† Ø§Ù„Ø±Ø§Ø¨Ø· ðŸ“', 'success');
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
          showToast('Ø®Ø§ØµÙŠØ© ØªØ­Ø¯ÙŠØ¯ Ø§Ù„Ù…ÙˆÙ‚Ø¹ ØºÙŠØ± Ù…Ø¯Ø¹ÙˆÙ…Ø© ÙÙŠ Ù…ØªØµÙØ­Ùƒ.', 'warning');
          return;
        }

        locateBtn.disabled = true;
        locateBtn.innerHTML = '<span>â³ Ø¬Ø§Ø±Ù Ø§Ù„ØªØ­Ø¯ÙŠØ¯ Ø¨Ø¯Ù‚Ø©...</span>';

        navigator.geolocation.getCurrentPosition(
          (pos) => {
            locateBtn.disabled = false;
            locateBtn.innerHTML = '<span>ðŸ“ Ù…ÙˆÙ‚Ø¹ÙŠ Ø§Ù„Ø­Ø§Ù„ÙŠ</span>';
            const userLat = pos.coords.latitude;
            const userLon = pos.coords.longitude;
            updateCoordinates(userLat, userLon, 16);
            const nearest = syncCityDropdownsWithCoordinates(userLat, userLon);
            if (nearest) {
              showToast(`ØªÙ… ØªØ­Ø¯ÙŠØ¯ Ù…ÙˆÙ‚Ø¹Ùƒ Ø¨Ø¯Ù‚Ø© Ø¹Ø§Ù„ÙŠØ© ÙˆØªØ­Ø¯ÙŠØ« Ø§Ù„Ù…Ø­Ø§ÙØ¸Ø© Ø§Ù„ØªØ§Ø¨Ø¹Ø© Ø¥Ù„Ù‰: ${nearest.name} ðŸ“`, 'success');
            } else {
              showToast('ØªÙ… ØªØ­Ø¯ÙŠØ¯ Ù…ÙˆÙ‚Ø¹Ùƒ Ø§Ù„Ø¬ØºØ±Ø§ÙÙŠ Ø¨Ø¯Ù‚Ø©.', 'success');
            }
          },
          (err) => {
            locateBtn.disabled = false;
            locateBtn.innerHTML = '<span>ðŸ“ Ù…ÙˆÙ‚Ø¹ÙŠ Ø§Ù„Ø­Ø§Ù„ÙŠ</span>';
            console.warn('Geolocation error:', err.message);
            showToast('ØªØ¹Ø°Ø± Ø¬Ù„Ø¨ Ù…ÙˆÙ‚Ø¹ GPS. ÙŠØ±Ø¬Ù‰ ØªÙØ¹ÙŠÙ„ Ø¥Ø°Ù† Ø§Ù„ÙˆØµÙˆÙ„ Ù„Ù„Ù…ÙˆÙ‚Ø¹ ÙÙŠ Ø§Ù„Ù…ØªØµÙØ­.', 'warning');
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


if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => {
    initMapPickers();
  });
} else {
  initMapPickers();
}
