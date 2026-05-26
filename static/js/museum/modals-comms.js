'use strict';

Modals.Locations = (() => {

        let geoData = [];
        let fbData = [];
        let mapViewInitialized = false;
        let _suppressLayerEvents = false;

        let mapView = null;
        let photoMarkersLayer = null;
        let whatsappMarkersLayer = null;
        let emailMarkersLayer = null;
        let messageMarkersLayer = null;
        let biographyMarkersLayer = null;
        let fbMarkersLayer = null;
        let otherMarkersLayer = null;
        let darkBlueMarker = null;
        let layerControl = null;

        let fbMarkers = [];
        let gpsCountBySource = [];
        let sourceMap = new Map();

        const KNOWN_GPS_SOURCES = new Set([
            'filesystem', 'biography', 'facebook_album', 'whatsapp',
            'email_attachment', 'gmail_attachment',
            'message', 'imessage', 'sms', 'message_attachment',
        ]);

        function _resetSourceMapCounts() {
            KNOWN_GPS_SOURCES.forEach(src => sourceMap.set(src, 0));
            sourceMap.set('other', 0);
        }
        _resetSourceMapCounts();

        const SHUFFLE_MARKER_LIMIT = 500;
        const DEFAULT_MARKER_LIMIT = 1000;

        const REGION_LABELS = {
            aus: 'Australia',
            dxb: 'Dubai',
            ireland: 'Ireland',
            uk: 'United Kingdom',
            scandinavia: 'Scandinavia',
            eur: 'Europe',
            canada: 'Canada',
            usa: 'USA',
            af: 'Africa',
            me: 'Middle East',
            malaysia: 'Malaysia',
            indonesia: 'Indonesia',
            india: 'India',
            thailand: 'Thailand',
            asia: 'Asia',
            central_america: 'Central America',
            carribean: 'Caribbean',
            nz: 'New Zealand',
            south_america: 'South America',
            oth: 'Other',
        };

        let selectedRegion = '';
        let regionSelectorBound = false;

        function _regionLabel(code) {
            if (code == null || code === '') return 'Unknown';
            const k = String(code).toLowerCase().trim();
            return REGION_LABELS[k] || code;
        }

        function _isRegionFilterActive() {
            return selectedRegion != null && String(selectedRegion).trim() !== '';
        }

        function _findFacebookLayerControlInput() {
            if (!layerControl || !layerControl._container) return null;
            const labels = layerControl._container.querySelectorAll('.leaflet-control-layers-overlays label');
            for (const label of labels) {
                if ((label.textContent || '').includes('Facebook Locations')) {
                    return label.querySelector('input[type="checkbox"]');
                }
            }
            return null;
        }

        function _syncFacebookLayerForRegionFilter() {
            if (!mapView || !fbMarkersLayer) return;
            const input = _findFacebookLayerControlInput();
            const label = input && input.closest('label');
            const regionActive = _isRegionFilterActive();

            _suppressLayerEvents = true;
            if (regionActive) {
                if (mapView.hasLayer(fbMarkersLayer)) {
                    mapView.removeLayer(fbMarkersLayer);
                }
                _setLayerMarkers(fbMarkersLayer, []);
                if (input) {
                    input.checked = false;
                    input.disabled = true;
                }
                if (label) {
                    label.classList.add('geo-layer-disabled');
                    label.title = 'Hidden while a region filter is active';
                }
            } else if (input) {
                input.disabled = false;
                if (label) {
                    label.classList.remove('geo-layer-disabled');
                    label.removeAttribute('title');
                }
            }
            _suppressLayerEvents = false;
        }

        function _bindRegionSelector(sel) {
            if (!sel || regionSelectorBound) return;
            regionSelectorBound = true;
            sel.addEventListener('change', () => {
                selectedRegion = sel.value;
                _syncFacebookLayerForRegionFilter();
                _reloadLocationDataForCurrentFilters();
            });
        }

        function _setRegionSelectorOptions(sel, regions, preserveSelection) {
            const prev = preserveSelection ? (sel.value || selectedRegion) : '';
            sel.innerHTML = '';
            const allOpt = document.createElement('option');
            allOpt.value = '';
            allOpt.textContent = 'All regions';
            sel.appendChild(allOpt);
            (regions || []).forEach(row => {
                const code = row && row.region != null ? String(row.region).trim() : '';
                if (!code) return;
                const o = document.createElement('option');
                o.value = code;
                const count = row.count != null ? Number(row.count) : 0;
                o.textContent = count > 0
                    ? `${_regionLabel(code)} (${count.toLocaleString()})`
                    : _regionLabel(code);
                sel.appendChild(o);
            });
            if (prev && Array.from(sel.options).some(o => o.value === prev)) {
                sel.value = prev;
                selectedRegion = prev;
            } else {
                sel.value = '';
                selectedRegion = '';
            }
        }

        function _getRegionSelectEl() {
            return DOM.geoMetadataRegionSelect
                || document.getElementById('geo-metadata-region-select')
                || (DOM.geoMetadataModal && DOM.geoMetadataModal.querySelector('#geo-metadata-region-select'));
        }

        async function _fetchGPSRegions() {
            const response = await fetch('/images/gps-regions', { cache: 'no-store' });
            if (!response.ok) {
                throw new Error(`Failed to fetch GPS regions: ${response.status}`);
            }
            const data = await response.json();
            return Array.isArray(data.regions) ? data.regions : [];
        }

        async function _populateRegionSelector(preserveSelection) {
            let regions = [];
            try {
                regions = await _fetchGPSRegions();
            } catch (error) {
                console.error('Error loading GPS regions for map filter:', error);
                regions = Object.keys(REGION_LABELS).map(code => ({ region: code, count: 0 }));
            }

            const sel = _getRegionSelectEl();
            if (!sel) {
                console.warn('Locations map region selector element not found');
                return regions;
            }
            _bindRegionSelector(sel);
            _setRegionSelectorOptions(sel, regions, preserveSelection !== false);
            _syncFacebookLayerForRegionFilter();
            return regions;
        }

        async function _reloadLocationDataForCurrentFilters(options = {}) {
            if (!mapView || !layerControl) {
                await _loadLocationData(['all'], null, options);
                return;
            }
            const { activeMap, categories } = getActiveLayerMap();
            await _loadLocationData(
                categories.length > 0 ? categories : ['all'],
                activeMap,
                options
            );
        }

        function init() {
            if (DOM.closeGeoMetadataModalBtn) DOM.closeGeoMetadataModalBtn.addEventListener('click', close);
            if (DOM.shufflePhotosBtn) DOM.shufflePhotosBtn.addEventListener('click', shufflePhotoMarkers);
        }

        async function open() {
            if (!DOM.geoMetadataModal) {
                console.error('Locations map modal element not found');
                return;
            }
            Modals._openModal(DOM.geoMetadataModal);
            await _populateRegionSelector(true);
            if (!mapView || !layerControl) {
                await _loadLocationData(['all'], null, { region: selectedRegion });
                return;
            }
            const { activeMap, categories } = getActiveLayerMap();
            await _loadLocationData(categories.length > 0 ? categories : ['all'], activeMap, { region: selectedRegion });
        }

        function close() {
            Modals._closeModal(DOM.geoMetadataModal);
        }

        function openMapView() {
            void open();
        }

        async function shufflePhotoMarkers() {
            if (!mapView || !layerControl) return;
            const { activeMap, categories } = getActiveLayerMap();
            if (categories.length === 0) return;
            await _loadLocationData(categories, activeMap, { limit: SHUFFLE_MARKER_LIMIT, region: selectedRegion });
        }

        function _sourceToActiveCategory(source) {
            const s = (source || '').toLowerCase();
            switch (s) {
                case 'filesystem':
                    return 'filesystem';
                case 'biography':
                    return 'biography';
                case 'whatsapp':
                    return 'whatsapp';
                case 'email_attachment':
                case 'gmail_attachment':
                    return 'email';
                case 'message':
                case 'imessage':
                case 'sms':
                case 'message_attachment':
                    return 'message';
                default:
                    return 'other';
            }
        }

        function _updateSourceMapFromCounts() {
            _resetSourceMapCounts();
            let otherTotal = 0;
            gpsCountBySource.forEach(item => {
                const src = item.source || '';
                if (KNOWN_GPS_SOURCES.has(src)) {
                    sourceMap.set(src, item.count);
                } else {
                    otherTotal += item.count;
                }
            });
            sourceMap.set('other', otherTotal);
        }

        function _layerLabelCount(category) {
            switch (category) {
                case 'filesystem':
                    return sourceMap.get('filesystem') || 0;
                case 'whatsapp':
                    return sourceMap.get('whatsapp') || 0;
                case 'email':
                    return (sourceMap.get('email_attachment') || 0) + (sourceMap.get('gmail_attachment') || 0);
                case 'message':
                    return (sourceMap.get('message') || 0)
                        + (sourceMap.get('imessage') || 0)
                        + (sourceMap.get('sms') || 0)
                        + (sourceMap.get('message_attachment') || 0);
                case 'biography':
                    return sourceMap.get('biography') || 0;
                case 'other':
                    return sourceMap.get('other') || 0;
                default:
                    return 0;
            }
        }

        async function _handleMarkerClick(item, allowRedirects = false) {
            try {
                const response = await fetch(`/images/${item.id}/metadata`);
                if (!response.ok) {
                    throw new Error(`Failed to fetch image metadata: ${response.status}`);
                }
                const fullImageData = await response.json();
                Modals.ImageDetailModal.open(fullImageData, {
                    allowRedirects: allowRedirects
                });
            } catch (error) {
                console.error('Error fetching image metadata:', error);
                const imageUrl = `/images/${item.id}?type=metadata`;
                const filename = item.title || item.source_reference || `Image ${item.id}`;
                Modals.SingleImageDisplay.showSingleImageModal(
                    filename,
                    imageUrl,
                    item.created_at,
                    item.latitude,
                    item.longitude
                );
            }
        }

        function _buildMarker(item, allowRedirects = false) {
            const marker = L.marker([item.latitude, item.longitude], { icon: darkBlueMarker });
            marker.on('click', function() {
                _handleMarkerClick(item, allowRedirects);
            });
            if (item.destination) {
                marker.bindPopup(item.destination);
            }
            return marker;
        }

        function _setLayerMarkers(layer, markers) {
            if (!layer) return;
            layer.clearLayers();
            markers.forEach(marker => layer.addLayer(marker));
        }

        function getActiveLayerMap() {
            const defaultActiveMap = {
                filesystem: true,
                whatsapp: true,
                email: true,
                message: true,
                biography: true,
                facebook: true,
                other: true,
            };
            if (!mapView || !layerControl) {
                return { activeMap: defaultActiveMap, categories: ['all'] };
            }

            const activeMap = {};
            const layers = layerControl._layers || {};
            for (const layerId of Object.keys(layers)) {
                const { layer, name, overlay } = layers[layerId];
                if (!overlay) continue;

                const cleanName0 = name.replace(/\([^)]*\)/, '');
                const cleanName = cleanName0.replace(/\s+/g, '');
                switch (cleanName) {
                    case 'GPSPhotosLocations':
                        activeMap.filesystem = mapView.hasLayer(layer);
                        break;
                    case 'WhatsAppLocations':
                        activeMap.whatsapp = mapView.hasLayer(layer);
                        break;
                    case 'EmailLocations':
                        activeMap.email = mapView.hasLayer(layer);
                        break;
                    case 'MessageLocations':
                        activeMap.message = mapView.hasLayer(layer);
                        break;
                    case 'BiographyLocations':
                        activeMap.biography = mapView.hasLayer(layer);
                        break;
                    case 'FacebookLocations':
                        activeMap.facebook = _isRegionFilterActive()
                            ? false
                            : mapView.hasLayer(layer);
                        break;
                    case 'OtherLocations':
                        activeMap.other = mapView.hasLayer(layer);
                        break;
                }
            }

            const categories = Object.keys(activeMap).filter(key => activeMap[key] && key !== 'facebook');
            return { activeMap, categories };
        }

        async function _loadLocationData(categories, activeMapOverride, options = {}) {
            const limit = options.limit || DEFAULT_MARKER_LIMIT;
            const region = options.region != null ? options.region : selectedRegion;
            const regionActive = region != null && String(region).trim() !== '';
            try {
                const placesPromise = regionActive
                    ? Promise.resolve({ places: [] })
                    : fetch('/facebook/places').then(r => r.json());
                const [locationsData, placesData, gpsData] = await Promise.all([
                    fetch('/getLocations/random', {
                        method: 'POST',
                        cache: 'no-store',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ categories, limit, region, _nonce: Date.now() }),
                    }).then(r => {
                        if (!r.ok) {
                            throw new Error(`Failed to fetch random locations: ${r.status}`);
                        }
                        return r.json();
                    }),
                    placesPromise,
                    fetch('/images/gps-count-by-source').then(r => r.json()),
                ]);

                geoData = locationsData.locations || [];
                fbData = placesData.places || [];
                gpsCountBySource = gpsData.by_source || [];
                _updateSourceMapFromCounts();

                _ensureMapView();

                const activeMap = activeMapOverride || getActiveLayerMap().activeMap;
                if (categories.length === 0) {
                    _clearMediaMarkers(activeMap);
                    _syncFacebookLayerForRegionFilter();
                    return;
                }
                _applyLocationsToMap(activeMap);
                _syncFacebookLayerForRegionFilter();
            } catch (error) {
                console.error('Error loading location data:', error);
            }
        }

        function _clearMediaMarkers(activeMap) {
            _setLayerMarkers(photoMarkersLayer, []);
            _setLayerMarkers(whatsappMarkersLayer, []);
            _setLayerMarkers(emailMarkersLayer, []);
            _setLayerMarkers(messageMarkersLayer, []);
            _setLayerMarkers(biographyMarkersLayer, []);
            _setLayerMarkers(otherMarkersLayer, []);

            fbMarkers = [];
            const showFacebook = activeMap && activeMap.facebook && !_isRegionFilterActive();
            if (showFacebook) {
                fbData.forEach(item => {
                    if (!item.latitude || !item.longitude) return;
                    const marker = L.marker([item.latitude, item.longitude], { icon: darkBlueMarker });
                    marker.bindPopup(item.name || 'Facebook Place');
                    fbMarkers.push(marker);
                });
                _setLayerMarkers(fbMarkersLayer, fbMarkers);
            } else {
                _setLayerMarkers(fbMarkersLayer, []);
            }

            const shownCount = document.getElementById('geo-metadata-shown-count');
            if (shownCount) {
                const shown = showFacebook ? fbMarkers.length : 0;
                shownCount.textContent = shown > 0
                    ? `Showing ${shown} GPS locations for selected layers`
                    : 'No GPS location layers selected';
            }
            if (mapView) {
                mapView.invalidateSize();
            }
        }

        function _applyLocationsToMap(activeMap) {
            const buckets = {
                filesystem: [],
                whatsapp: [],
                email: [],
                message: [],
                biography: [],
                other: [],
            };
            const latlngs = [];

            geoData.forEach(item => {
                if (!item.latitude || !item.longitude) return;
                const category = _sourceToActiveCategory(item.source);
                if (!buckets[category]) return;
                buckets[category].push(_buildMarker(item, category === 'email'));
                if (activeMap[category]) {
                    latlngs.push([item.latitude, item.longitude]);
                }
            });

            fbMarkers = [];
            const showFacebook = activeMap.facebook && !_isRegionFilterActive();
            fbData.forEach(item => {
                if (!item.latitude || !item.longitude) return;
                const marker = L.marker([item.latitude, item.longitude], { icon: darkBlueMarker });
                marker.bindPopup(item.name || 'Facebook Place');
                fbMarkers.push(marker);
                if (showFacebook) {
                    latlngs.push([item.latitude, item.longitude]);
                }
            });

            _setLayerMarkers(photoMarkersLayer, activeMap.filesystem ? buckets.filesystem : []);
            _setLayerMarkers(whatsappMarkersLayer, activeMap.whatsapp ? buckets.whatsapp : []);
            _setLayerMarkers(emailMarkersLayer, activeMap.email ? buckets.email : []);
            _setLayerMarkers(messageMarkersLayer, activeMap.message ? buckets.message : []);
            _setLayerMarkers(biographyMarkersLayer, activeMap.biography ? buckets.biography : []);
            _setLayerMarkers(otherMarkersLayer, activeMap.other ? buckets.other : []);
            _setLayerMarkers(fbMarkersLayer, showFacebook ? fbMarkers : []);

            let shown = 0;
            if (activeMap.filesystem) shown += buckets.filesystem.length;
            if (activeMap.whatsapp) shown += buckets.whatsapp.length;
            if (activeMap.email) shown += buckets.email.length;
            if (activeMap.message) shown += buckets.message.length;
            if (activeMap.biography) shown += buckets.biography.length;
            if (activeMap.other) shown += buckets.other.length;
            if (showFacebook) shown += fbMarkers.length;

            const shownCount = document.getElementById('geo-metadata-shown-count');
            if (shownCount) {
                shownCount.textContent = `Showing ${shown} GPS locations for selected layers`;
            }

            if (latlngs.length > 0) {
                mapView.fitBounds(latlngs, { padding: [20, 20] });
            }
            mapView.invalidateSize();
        }

        async function layerFiltersChanged() {
            if (!mapView || !layerControl || _suppressLayerEvents) return;
            if (_isRegionFilterActive()) {
                _syncFacebookLayerForRegionFilter();
            }
            const { activeMap, categories } = getActiveLayerMap();
            if (categories.length === 0) {
                _clearMediaMarkers(activeMap);
                return;
            }
            await _loadLocationData(categories, activeMap, { region: selectedRegion });
        }

        function _ensureMapView() {
            if (mapViewInitialized) {
                setTimeout(() => { mapView.invalidateSize(); }, 100);
                return;
            }

            mapView = L.map('map-view', {
                minZoom: 1,
                maxZoom: 19,
            });
            L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
                maxZoom: 19,
                attribution: '&copy; <a href="http://www.openstreetmap.org/copyright">OpenStreetMap</a>'
            }).addTo(mapView);

            layerControl = L.control.layers().addTo(mapView);

            darkBlueMarker = L.icon({
                iconUrl: '/static/images/marker-dark-blue.png',
                iconSize: [25, 35],
                iconAnchor: [12, 32],
                popupAnchor: [0, -32]
            });

            _suppressLayerEvents = true;

            photoMarkersLayer = L.layerGroup().addTo(mapView);
            layerControl.addOverlay(photoMarkersLayer, 'GPS Photos Locations (' + _layerLabelCount('filesystem') + ')');

            whatsappMarkersLayer = L.layerGroup().addTo(mapView);
            layerControl.addOverlay(whatsappMarkersLayer, 'WhatsApp Locations (' + _layerLabelCount('whatsapp') + ')');

            emailMarkersLayer = L.layerGroup().addTo(mapView);
            layerControl.addOverlay(emailMarkersLayer, 'Email Locations (' + _layerLabelCount('email') + ')');

            messageMarkersLayer = L.layerGroup().addTo(mapView);
            layerControl.addOverlay(messageMarkersLayer, 'Message Locations (' + _layerLabelCount('message') + ')');

            biographyMarkersLayer = L.layerGroup().addTo(mapView);
            layerControl.addOverlay(biographyMarkersLayer, 'Biography Locations (' + _layerLabelCount('biography') + ')');

            fbMarkersLayer = L.layerGroup().addTo(mapView);
            layerControl.addOverlay(fbMarkersLayer, 'Facebook Locations (' + fbData.length + ')');

            otherMarkersLayer = L.layerGroup().addTo(mapView);
            layerControl.addOverlay(otherMarkersLayer, 'Other Locations (' + _layerLabelCount('other') + ')');

            _suppressLayerEvents = false;

            _syncFacebookLayerForRegionFilter();

            mapView.on('overlayadd overlayremove baselayerchange', function() {
                layerFiltersChanged();
            });

            mapViewInitialized = true;
            setTimeout(() => { mapView.invalidateSize(); }, 1000);
        }

         return { init, open, openMapView, shufflePhotoMarkers };
})();


Modals.NearbyLocations = (() => {
        let mapView = null;
        let markersLayer = null;
        let mapInitialized = false;
        let darkBlueMarker = null;
        let centerMarker = null;
        let centerLat = null;
        let centerLng = null;
        let centerImageId = null;
        let currentRadiusKm = 25;
        let loadToken = 0;
        const NEARBY_MARKER_LIMIT = 1000;

        function init() {
            if (DOM.closeNearbyLocationsModalBtn) {
                DOM.closeNearbyLocationsModalBtn.addEventListener('click', close);
            }
            if (DOM.nearbyRadiusUpdateBtn) {
                DOM.nearbyRadiusUpdateBtn.addEventListener('click', () => { void _applyRadiusFromControl(); });
            }
            if (DOM.nearbyRadiusInput) {
                DOM.nearbyRadiusInput.addEventListener('keydown', (e) => {
                    if (e.key === 'Enter') {
                        e.preventDefault();
                        void _applyRadiusFromControl();
                    }
                });
            }
        }

        function close() {
            Modals._closeModal(DOM.nearbyLocationsModal);
        }

        function _parseRadius(raw) {
            const radiusKm = parseFloat(String(raw).trim());
            if (!Number.isFinite(radiusKm) || radiusKm <= 0 || radiusKm > 500) {
                return null;
            }
            return radiusKm;
        }

        function _syncRadiusControl() {
            if (DOM.nearbyRadiusInput) {
                DOM.nearbyRadiusInput.value = String(currentRadiusKm);
            }
        }

        function _updateLimitNotice(total) {
            if (!DOM.nearbyLocationsLimitNotice) return;
            if (total === NEARBY_MARKER_LIMIT) {
                DOM.nearbyLocationsLimitNotice.textContent = `Showing Nearest ${NEARBY_MARKER_LIMIT} Locations`;
                DOM.nearbyLocationsLimitNotice.style.display = 'block';
                return;
            }
            DOM.nearbyLocationsLimitNotice.textContent = '';
            DOM.nearbyLocationsLimitNotice.style.display = 'none';
        }

        function _restoreModalShell() {
            if (!DOM.nearbyLocationsModal) return;
            DOM.nearbyLocationsModal.querySelectorAll('.modal-content, .modal-header, .geo-metadata-tab-content').forEach(el => {
                if (el.style.display === 'none') {
                    el.style.display = '';
                }
            });
        }

        async function _loadNearbyLocations() {
            if (centerLat == null || centerLng == null) return;
            const token = ++loadToken;
            _updateLimitNotice(0);
            if (DOM.nearbyLocationsCount) {
                DOM.nearbyLocationsCount.textContent = 'Loading nearby locations…';
            }

            try {
                const response = await fetch('/images/locations/nearby', {
                    method: 'POST',
                    cache: 'no-store',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        latitude: centerLat,
                        longitude: centerLng,
                        radius_km: currentRadiusKm,
                        limit: NEARBY_MARKER_LIMIT,
                    }),
                });
                if (token !== loadToken) return;
                if (!response.ok) {
                    throw new Error(`Failed to fetch nearby locations: ${response.status}`);
                }
                const data = await response.json();
                if (token !== loadToken) return;
                const locations = data.locations || [];
                _ensureMap();
                _renderMarkers(locations, centerImageId);
                const total = data.total != null ? data.total : locations.length;
                _updateLimitNotice(total);
                if (DOM.nearbyLocationsCount) {
                    DOM.nearbyLocationsCount.textContent = `${total} image${total === 1 ? '' : 's'} within ${currentRadiusKm} km`;
                }
            } catch (error) {
                if (token !== loadToken) return;
                console.error('Error loading nearby locations:', error);
                _updateLimitNotice(0);
                if (DOM.nearbyLocationsCount) {
                    DOM.nearbyLocationsCount.textContent = 'Failed to load nearby locations';
                }
            }
        }

        async function _applyRadiusFromControl() {
            if (!DOM.nearbyRadiusInput) return;
            const parsed = _parseRadius(DOM.nearbyRadiusInput.value);
            if (parsed == null) {
                await AppDialogs.showAppAlert('Invalid radius', 'Enter a value between 0 and 500 km.');
                _syncRadiusControl();
                return;
            }
            currentRadiusKm = parsed;
            _syncRadiusControl();
            await _loadNearbyLocations();
        }

        async function open(centerLatArg, centerLngArg, radiusKm, centerImageIdArg) {
            centerLat = centerLatArg;
            centerLng = centerLngArg;
            centerImageId = centerImageIdArg;
            currentRadiusKm = _parseRadius(radiusKm) || 25;

            if (typeof Modals.closeAll === 'function') {
                Modals.closeAll({ keepOpen: DOM.nearbyLocationsModal });
            }
            Modals._openModal(DOM.nearbyLocationsModal);
            _restoreModalShell();
            _syncRadiusControl();
            await _loadNearbyLocations();
        }

        async function _handleMarkerClick(item) {
            try {
                const response = await fetch(`/images/${item.id}/metadata`);
                if (!response.ok) {
                    throw new Error(`Failed to fetch image metadata: ${response.status}`);
                }
                const fullImageData = await response.json();
                Modals.ImageDetailModal.open(fullImageData);
            } catch (error) {
                console.error('Error fetching image metadata:', error);
                await AppDialogs.showAppAlert('Error', 'Unable to open image details.');
            }
        }

        function _ensureMap() {
            if (mapInitialized) {
                setTimeout(() => { mapView.invalidateSize(); }, 100);
                return;
            }

            mapView = L.map('nearby-map-view', {
                minZoom: 1,
                maxZoom: 19,
            });
            L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
                maxZoom: 19,
                attribution: '&copy; <a href="http://www.openstreetmap.org/copyright">OpenStreetMap</a>'
            }).addTo(mapView);

            darkBlueMarker = L.icon({
                iconUrl: '/static/images/marker-dark-blue.png',
                iconSize: [25, 35],
                iconAnchor: [12, 32],
                popupAnchor: [0, -32]
            });

            centerMarker = L.divIcon({
                className: 'nearby-center-marker',
                html: '<div style="background:#c0392b;width:14px;height:14px;border-radius:50%;border:2px solid #fff;box-shadow:0 0 4px rgba(0,0,0,0.45);"></div>',
                iconSize: [18, 18],
                iconAnchor: [9, 9],
            });

            markersLayer = L.layerGroup().addTo(mapView);
            mapInitialized = true;
            setTimeout(() => { mapView.invalidateSize(); }, 100);
        }

        function _renderMarkers(locations, centerImageId) {
            if (!mapView || !markersLayer) return;
            markersLayer.clearLayers();
            const latlngs = [];

            locations.forEach(item => {
                if (!item.latitude || !item.longitude) return;
                const isCenter = centerImageId != null && item.id === centerImageId;
                const icon = isCenter ? centerMarker : darkBlueMarker;
                const marker = L.marker([item.latitude, item.longitude], { icon });
                marker.on('click', () => _handleMarkerClick(item));
                markersLayer.addLayer(marker);
                latlngs.push([item.latitude, item.longitude]);
            });

            if (latlngs.length > 0) {
                mapView.fitBounds(latlngs, { padding: [20, 20] });
            }
            mapView.invalidateSize();
        }

        return { init, open, close };
})();


Modals.ConversationSummary = (() => {
        let currentChatSession = null;

        const DEFAULT_SUMMARY_TITLE_HTML = '<i class="fas fa-file-lines"></i> Conversation Summary';
        const ASK_AI_MODAL_TITLE_HTML = '<i class="fas fa-robot"></i> Ask the AI';

        function setSummaryModalTitle(html) {
            const modal = document.getElementById('conversation-summary-modal');
            const h2 = modal && modal.querySelector('.modal-header h2');
            if (h2) {
                h2.innerHTML = html;
            }
        }

        function open(chatSession) {
            currentChatSession = chatSession;
            setSummaryModalTitle(DEFAULT_SUMMARY_TITLE_HTML);
            const modal = document.getElementById('conversation-summary-modal');
            const progressDiv = document.getElementById('conversation-summary-progress');
            const contentDiv = document.getElementById('conversation-summary-content');
            const errorDiv = document.getElementById('conversation-summary-error');
            const progressText = document.getElementById('conversation-summary-progress-text');

            if (!modal) {
                console.error('Conversation summary modal not found');
                return;
            }

            // Reset UI and show waiting dialog immediately
            contentDiv.innerHTML = '';
            errorDiv.style.display = 'none';
            errorDiv.innerHTML = '';
            progressDiv.style.display = 'flex';
            progressText.textContent = 'Generating summary... Please wait.';
            modal.style.display = 'flex';

            // Encode chat session for URL
            const encodedSession = encodeURIComponent(chatSession);

            // Use requestAnimationFrame to ensure modal is visible before starting fetch
            requestAnimationFrame(() => {
                // Call summarize endpoint synchronously
                fetch(`/imessages/conversation/${encodedSession}/summarize`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                })
                .then(response => {
                    if (!response.ok) {
                        return response.json().then(err => {
                            const errorMsg = err.detail || 'Failed to generate summary';
                            throw new Error(errorMsg);
                        }).catch(() => {
                            // If JSON parsing fails, use status text
                            throw new Error(`Server error: ${response.status} ${response.statusText}`);
                        });
                    }
                    return response.json();
                })
                .then(data => {
                    // Display summary
                    if ( data.summary && data.summary.length > 0) {
                        displaySummary(data.summary);
                    } else {
                        displayError('Unexpected response format');
                    }
                })
                .catch(error => {
                    let errorMessage = 'Failed to generate summary';
                    
                    if (error.message) {
                        errorMessage = error.message;
                    } else if (error instanceof TypeError && error.message.includes('fetch')) {
                        errorMessage = 'Network error: Unable to connect to server. Please check your connection.';
                    } else if (error.name === 'NetworkError' || error.message.includes('network')) {
                        errorMessage = 'Network error: Unable to connect to server. Please check your connection.';
                    }
                    
                    displayError(errorMessage);
                });
            });
        }

        function openForEmailThread(chatSession) {
            currentChatSession = chatSession;
            setSummaryModalTitle(DEFAULT_SUMMARY_TITLE_HTML);
            const modal = document.getElementById('conversation-summary-modal');
            const progressDiv = document.getElementById('conversation-summary-progress');
            const contentDiv = document.getElementById('conversation-summary-content');
            const errorDiv = document.getElementById('conversation-summary-error');
            const progressText = document.getElementById('conversation-summary-progress-text');

            if (!modal) {
                console.error('Conversation summary modal not found');
                return;
            }

            // Reset UI and show waiting dialog immediately
            contentDiv.innerHTML = '';
            errorDiv.style.display = 'none';
            errorDiv.innerHTML = '';
            progressDiv.style.display = 'flex';
            progressText.textContent = 'Generating summary... Please wait.';
            modal.style.display = 'flex';

            // Encode chat session for URL
            const encodedSession = encodeURIComponent(chatSession);

            // Use requestAnimationFrame to ensure modal is visible before starting fetch
            requestAnimationFrame(() => {
                // Call summarize endpoint synchronously
                fetch(`/emails/thread/${encodedSession}/summarize`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                })
                .then(response => {
                    if (!response.ok) {
                        return response.json().then(err => {
                            const errorMsg = err.detail || 'Failed to generate summary';
                            throw new Error(errorMsg);
                        }).catch(() => {
                            // If JSON parsing fails, use status text
                            throw new Error(`Server error: ${response.status} ${response.statusText}`);
                        });
                    }
                    return response.json();
                })
                .then(data => {
                    // Display summary
                    if ( data.summary && data.summary.length > 0) {
                        displaySummary(data.summary);
                    } else {
                        displayError('Unexpected response format');
                    }
                })
                .catch(error => {
                    let errorMessage = 'Failed to generate summary';
                    
                    if (error.message) {
                        errorMessage = error.message;
                    } else if (error instanceof TypeError && error.message.includes('fetch')) {
                        errorMessage = 'Network error: Unable to connect to server. Please check your connection.';
                    } else if (error.name === 'NetworkError' || error.message.includes('network')) {
                        errorMessage = 'Network error: Unable to connect to server. Please check your connection.';
                    }
                    
                    displayError(errorMessage);
                });
            });
        }

        /** Same UI as summarize; POST /imessages/conversation/.../prompt with { instruction }. */
        function openWithIMessagePrompt(chatSession, instruction) {
            currentChatSession = chatSession;
            setSummaryModalTitle(ASK_AI_MODAL_TITLE_HTML);
            const modal = document.getElementById('conversation-summary-modal');
            const progressDiv = document.getElementById('conversation-summary-progress');
            const contentDiv = document.getElementById('conversation-summary-content');
            const errorDiv = document.getElementById('conversation-summary-error');
            const progressText = document.getElementById('conversation-summary-progress-text');

            if (!modal) {
                console.error('Conversation summary modal not found');
                return;
            }

            contentDiv.innerHTML = '';
            errorDiv.style.display = 'none';
            errorDiv.innerHTML = '';
            progressDiv.style.display = 'flex';
            progressText.textContent = 'Working on your request... Please wait.';
            modal.style.display = 'flex';

            const encodedSession = encodeURIComponent(chatSession);

            requestAnimationFrame(() => {
                fetch(`/imessages/conversation/${encodedSession}/prompt`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ instruction: instruction })
                })
                    .then((response) => {
                        if (!response.ok) {
                            return response.json().then((err) => {
                                const errorMsg = err.detail || 'Failed to complete request';
                                throw new Error(errorMsg);
                            }).catch(() => {
                                throw new Error(`Server error: ${response.status} ${response.statusText}`);
                            });
                        }
                        return response.json();
                    })
                    .then((data) => {
                        if (data.response && data.response.length > 0) {
                            displaySummary(data.response);
                        } else {
                            displayError('Unexpected response format');
                        }
                    })
                    .catch((error) => {
                        let errorMessage = 'Failed to process your request';
                        if (error.message) {
                            errorMessage = error.message;
                        } else if (error instanceof TypeError && error.message.includes('fetch')) {
                            errorMessage = 'Network error: Unable to connect to server. Please check your connection.';
                        } else if (error.name === 'NetworkError' || (error.message && error.message.includes('network'))) {
                            errorMessage = 'Network error: Unable to connect to server. Please check your connection.';
                        }
                        displayError(errorMessage);
                    });
            });
        }

        /** Same UI; POST /emails/thread/.../prompt with { instruction } (participant = thread id, e.g. sender email). */
        function openWithEmailThreadPrompt(participant, instruction) {
            currentChatSession = participant;
            setSummaryModalTitle(ASK_AI_MODAL_TITLE_HTML);
            const modal = document.getElementById('conversation-summary-modal');
            const progressDiv = document.getElementById('conversation-summary-progress');
            const contentDiv = document.getElementById('conversation-summary-content');
            const errorDiv = document.getElementById('conversation-summary-error');
            const progressText = document.getElementById('conversation-summary-progress-text');

            if (!modal) {
                console.error('Conversation summary modal not found');
                return;
            }

            contentDiv.innerHTML = '';
            errorDiv.style.display = 'none';
            errorDiv.innerHTML = '';
            progressDiv.style.display = 'flex';
            progressText.textContent = 'Working on your request... Please wait.';
            modal.style.display = 'flex';

            const encodedParticipant = encodeURIComponent(participant);

            requestAnimationFrame(() => {
                fetch(`/emails/thread/${encodedParticipant}/prompt`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ instruction: instruction })
                })
                    .then((response) => {
                        if (!response.ok) {
                            return response.json().then((err) => {
                                const errorMsg = err.detail || 'Failed to complete request';
                                throw new Error(errorMsg);
                            }).catch(() => {
                                throw new Error(`Server error: ${response.status} ${response.statusText}`);
                            });
                        }
                        return response.json();
                    })
                    .then((data) => {
                        if (data.response && data.response.length > 0) {
                            displaySummary(data.response);
                        } else {
                            displayError('Unexpected response format');
                        }
                    })
                    .catch((error) => {
                        let errorMessage = 'Failed to process your request';
                        if (error.message) {
                            errorMessage = error.message;
                        } else if (error instanceof TypeError && error.message.includes('fetch')) {
                            errorMessage = 'Network error: Unable to connect to server. Please check your connection.';
                        } else if (error.name === 'NetworkError' || (error.message && error.message.includes('network'))) {
                            errorMessage = 'Network error: Unable to connect to server. Please check your connection.';
                        }
                        displayError(errorMessage);
                    });
            });
        }

        function displaySummary(summary) {
            const progressDiv = document.getElementById('conversation-summary-progress');
            const contentDiv = document.getElementById('conversation-summary-content');
            const errorDiv = document.getElementById('conversation-summary-error');

            if (!contentDiv) return;

            // Hide progress
            if (progressDiv) {
                progressDiv.style.display = 'none';
            }

            // Hide error
            if (errorDiv) {
                errorDiv.style.display = 'none';
            }

            // Display summary (support markdown-like formatting)
            contentDiv.innerHTML = formatSummaryText(summary);
        }

        function formatSummaryText(text) {
            if (!text) return '<p>No summary available.</p>';

            // Check if marked is available
            if (typeof marked !== 'undefined') {
                try {
                    // Parse and render Markdown
                    const html = marked.parse(text);
                    
                    // Sanitize HTML if DOMPurify is available
                    if (typeof DOMPurify !== 'undefined') {
                        return DOMPurify.sanitize(html);
                    }
                    
                    return html;
                } catch (error) {
                    console.error('Error rendering markdown:', error);
                    // Fallback to plain text if markdown parsing fails
                    return formatSummaryTextPlain(text);
                }
            } else {
                // Fallback if marked is not available
                console.warn('marked.js not available, using plain text formatting');
                return formatSummaryTextPlain(text);
            }
        }

        function formatSummaryTextPlain(text) {
            // Escape HTML first
            const escaped = text
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;');

            // Convert line breaks to paragraphs
            const paragraphs = escaped.split(/\n\n+/).filter(p => p.trim());
            return paragraphs.map(p => `<p>${p.replace(/\n/g, '<br>')}</p>`).join('');
        }

        function displayError(errorMessage) {
            const progressDiv = document.getElementById('conversation-summary-progress');
            const errorDiv = document.getElementById('conversation-summary-error');
            const contentDiv = document.getElementById('conversation-summary-content');

            if (!errorDiv) return;

            // Hide progress
            if (progressDiv) {
                progressDiv.style.display = 'none';
            }

            // Clear content
            if (contentDiv) {
                contentDiv.innerHTML = '';
            }

            // Display error
            errorDiv.style.display = 'block';
            errorDiv.innerHTML = `<strong>Error:</strong> ${escapeHtml(errorMessage)}`;
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        function close() {
            // Hide modal
            const modal = document.getElementById('conversation-summary-modal');
            if (modal) {
                modal.style.display = 'none';
            }

            setSummaryModalTitle(DEFAULT_SUMMARY_TITLE_HTML);
            currentChatSession = null;
        }

        function init() {
            const closeBtn = document.getElementById('close-conversation-summary-modal');
            const modal = document.getElementById('conversation-summary-modal');

            if (closeBtn) {
                closeBtn.addEventListener('click', () => {
                    close();
                });
            }

            // Close modal when clicking outside
            if (modal) {
                modal.addEventListener('click', (e) => {
                    if (e.target === modal) {
                        close();
                    }
                });
            }
        }

        return { init, open, openForEmailThread, openWithIMessagePrompt, openWithEmailThreadPrompt, close, displaySummary, displayError };
})();


Modals.SMSMessages = (() => {
        let chatSessions = [];
        let contactsSessions = [];
        let groupsSessions = [];
        let otherSessions = [];
        let filteredSessions = [];
        let originalChatData = null;
        let currentSession = null;
        let messageTypeFilters = {
            imessage: true,
            sms: true,
            whatsapp: true,
            facebook: true,
            instagram: true,
            mixed: true
        };

        function formatAustralianDate(dateString) {
            if (!dateString) return '';
            const date = new Date(dateString);
            const day = String(date.getDate()).padStart(2, '0');
            const month = String(date.getMonth() + 1).padStart(2, '0');
            const year = date.getFullYear();
            const hours = String(date.getHours()).padStart(2, '0');
            const minutes = String(date.getMinutes()).padStart(2, '0');
            const seconds = String(date.getSeconds()).padStart(2, '0');
            return `${day}/${month}/${year} ${hours}:${minutes}:${seconds}`;
        }

        async function loadChatSessions() {
            const listContainer = document.getElementById('sms-chat-sessions-list');
            if (!listContainer) return;

            listContainer.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666;">Loading conversations...</div>';

            try {
                const response = await fetch('/imessages/chat-sessions');
                if (!response.ok) {
                    throw new Error('Failed to load chat sessions');
                }
                const data = await response.json();
                // Store original data structure
                originalChatData = data;
                // Keep categories separate
                contactsSessions = data.contacts || [];
                groupsSessions = data.groups || [];
                otherSessions = data.other || [];
                // Combine for backward compatibility with search
                chatSessions = [
                    ...contactsSessions,
                    ...groupsSessions,
                    ...otherSessions
                ];
                filteredSessions = [...chatSessions];
                renderChatSessions();
            } catch (error) {
                console.error('Error loading chat sessions:', error);
                let errorMessage = 'Error loading conversations';
                if (error.message) {
                    errorMessage += ': ' + error.message;
                }
                listContainer.innerHTML = `<div style="text-align: center; padding: 2rem; color: #dc3545;">${errorMessage}</div>`;
            }
        }

        function renderChatSessions() {
            const listContainer = document.getElementById('sms-chat-sessions-list');
            if (!listContainer) return;

            // Helper function to filter sessions by message type
            function filterByMessageType(sessions) {
                return sessions.filter(session => {
                    const messageType = session.message_type;
                    
                    // If message type is not defined, don't show it
                    if (!messageType) {
                        return false;
                    }
                    
                    // If it's a mixed conversation, show it if ANY individual type is selected OR if mixed is selected
                    if (messageType === 'mixed') {
                        return messageTypeFilters.mixed === true || 
                               messageTypeFilters.imessage === true || 
                               messageTypeFilters.sms === true || 
                               messageTypeFilters.whatsapp === true || 
                               messageTypeFilters.facebook === true ||
                               messageTypeFilters.instagram === true;
                    }
                    
                    // For other types, check if the filter is enabled
                    // Strict boolean check since we're setting boolean values
                    return messageTypeFilters[messageType] === true;
                });
            }

            // Filter each category separately by message type
            // Also filter by search if filteredSessions is different from chatSessions
            // Create a Set of chat_session values from filteredSessions for efficient lookup
            const filteredSet = new Set(filteredSessions.map(s => s.chat_session));
            const isSearchActive = filteredSessions.length !== chatSessions.length;
            
            let filteredContacts = contactsSessions;
            let filteredGroups = groupsSessions;
            let filteredOther = otherSessions;
            
            if (isSearchActive) {
                // If search is active, filter each category by what's in filteredSessions
                filteredContacts = contactsSessions.filter(s => filteredSet.has(s.chat_session));
                filteredGroups = groupsSessions.filter(s => filteredSet.has(s.chat_session));
                filteredOther = otherSessions.filter(s => filteredSet.has(s.chat_session));
            }
            
            // Apply message type filter to each category
            const typeFilteredContacts = filterByMessageType(filteredContacts);
            const typeFilteredGroups = filterByMessageType(filteredGroups);
            const typeFilteredOther = filterByMessageType(filteredOther);

            const totalFiltered = typeFilteredContacts.length + typeFilteredGroups.length + typeFilteredOther.length;
            
            if (totalFiltered === 0) {
                listContainer.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666;">No conversations found</div>';
                return;
            }

            listContainer.innerHTML = '';
            
            // Render Contacts section
            if (typeFilteredContacts.length > 0) {
                const categoryHeader = document.createElement('div');
                categoryHeader.className = 'sms-chat-category-header';
                categoryHeader.textContent = 'Contacts';
                categoryHeader.style.cssText = 'padding: 12px 16px; font-weight: 600; font-size: 13px; color: #233366; background-color: #e9ecef; border-bottom: 1px solid #dee2e6; text-transform: uppercase; letter-spacing: 0.5px;';
                listContainer.appendChild(categoryHeader);
                
                typeFilteredContacts.forEach(session => {
                    renderChatSessionItem(session, listContainer);
                });
            }
            
            // Render Group Chats section
            if (typeFilteredGroups.length > 0) {
                const categoryHeader = document.createElement('div');
                categoryHeader.className = 'sms-chat-category-header';
                categoryHeader.textContent = 'Group Chats';
                const hasPreviousSection = typeFilteredContacts.length > 0;
                categoryHeader.style.cssText = 'padding: 12px 16px; font-weight: 600; font-size: 13px; color: #233366; background-color: #e9ecef; border-bottom: 1px solid #dee2e6; border-top: 1px solid #dee2e6; margin-top: ' + (hasPreviousSection ? '8px' : '0') + '; text-transform: uppercase; letter-spacing: 0.5px;';
                listContainer.appendChild(categoryHeader);
                
                typeFilteredGroups.forEach(session => {
                    renderChatSessionItem(session, listContainer);
                });
            }
            
            // Render Other section
            if (typeFilteredOther.length > 0) {
                const categoryHeader = document.createElement('div');
                categoryHeader.className = 'sms-chat-category-header';
                categoryHeader.textContent = 'Other';
                const hasPreviousSection = typeFilteredContacts.length > 0 || typeFilteredGroups.length > 0;
                categoryHeader.style.cssText = 'padding: 12px 16px; font-weight: 600; font-size: 13px; color: #233366; background-color: #e9ecef; border-bottom: 1px solid #dee2e6; border-top: 1px solid #dee2e6; margin-top: ' + (hasPreviousSection ? '8px' : '0') + '; text-transform: uppercase; letter-spacing: 0.5px;';
                listContainer.appendChild(categoryHeader);
                
                typeFilteredOther.forEach(session => {
                    renderChatSessionItem(session, listContainer);
                });
            }
        }

        function renderChatSessionItem(session, listContainer) {
                const item = document.createElement('div');
                item.className = 'sms-chat-session-item';
                item.dataset.session = session.chat_session;
                
                // Profile picture/avatar
                const avatar = document.createElement('div');
                avatar.className = 'sms-chat-session-avatar';
                const initials = (session.chat_session || 'U').substring(0, 2).toUpperCase();
                avatar.textContent = initials;
                item.appendChild(avatar);
                
                // Content container
                const content = document.createElement('div');
                content.className = 'sms-chat-session-content';
                
                // Header with name and time
                const header = document.createElement('div');
                header.className = 'sms-chat-session-header';
                
                const nameSpan = document.createElement('span');
                nameSpan.className = 'sms-chat-session-name';
                
                // Message type icon
                const messageTypeIcon = document.createElement('i');
                if (session.message_type === 'imessage') {
                    messageTypeIcon.className = 'fab fa-apple';
                    messageTypeIcon.title = 'iMessage';
                    messageTypeIcon.style.marginRight = '6px';
                    messageTypeIcon.style.color = '#007AFF';
                    messageTypeIcon.style.fontSize = '14px';
                } else if (session.message_type === 'sms') {
                    messageTypeIcon.className = 'fas fa-comment';
                    messageTypeIcon.title = 'SMS';
                    messageTypeIcon.style.marginRight = '6px';
                    messageTypeIcon.style.color = '#34C759';
                    messageTypeIcon.style.fontSize = '14px';
                } else if (session.message_type === 'whatsapp') {
                    messageTypeIcon.className = 'fab fa-whatsapp';
                    messageTypeIcon.title = 'WhatsApp';
                    messageTypeIcon.style.marginRight = '6px';
                    messageTypeIcon.style.color = '#25D366';
                    messageTypeIcon.style.fontSize = '14px';
                } else if (session.message_type === 'facebook') {
                    messageTypeIcon.className = 'fab fa-facebook-messenger';
                    messageTypeIcon.title = 'Facebook Messenger';
                    messageTypeIcon.style.marginRight = '6px';
                    messageTypeIcon.style.color = '#0084FF';
                    messageTypeIcon.style.fontSize = '14px';
                } else if (session.message_type === 'instagram') {
                    messageTypeIcon.className = 'fab fa-instagram';
                    messageTypeIcon.title = 'Instagram';
                    messageTypeIcon.style.marginRight = '6px';
                    messageTypeIcon.style.color = '#E4405F';
                    messageTypeIcon.style.fontSize = '14px';
                } else if (session.message_type === 'mixed') {
                    messageTypeIcon.className = 'fas fa-comments';
                    messageTypeIcon.title = 'Mixed';
                    messageTypeIcon.style.marginRight = '6px';
                    messageTypeIcon.style.color = '#FF9500';
                    messageTypeIcon.style.fontSize = '14px';
                }
                nameSpan.appendChild(messageTypeIcon);
                
                const nameText = document.createTextNode(session.chat_session || 'Unknown');
                nameSpan.appendChild(nameText);
                
                header.appendChild(nameSpan);
                
                // Remove time display from master pane - not showing anything
                const timeSpan = document.createElement('span');
                timeSpan.className = 'sms-chat-session-time';
                timeSpan.textContent = '';
                header.appendChild(timeSpan);
                
                content.appendChild(header);
                
                // Preview with attachment indicator
                const preview = document.createElement('div');
                preview.className = 'sms-chat-session-preview';
                
                const previewText = document.createElement('span');
                previewText.className = 'sms-chat-session-preview-text';
                
                // Attachment indicator
                if (session.has_attachments) {
                    const attachmentIcon = document.createElement('i');
                    attachmentIcon.className = 'fas fa-paperclip';
                    attachmentIcon.style.color = '#667781';
                    attachmentIcon.style.fontSize = '12px';
                    attachmentIcon.style.marginRight = '4px';
                    previewText.appendChild(attachmentIcon);
                }
                
                const previewTextNode = document.createTextNode(`${session.message_count || 0} message${session.message_count !== 1 ? 's' : ''}`);
                previewText.appendChild(previewTextNode);
                
                preview.appendChild(previewText);
                
                // Unread count badge
                if (session.message_count > 0) {
                    const countSpan = document.createElement('span');
                    countSpan.className = 'sms-chat-session-count';
                    countSpan.textContent = session.message_count > 99 ? '99+' : session.message_count;
                    preview.appendChild(countSpan);
                }
                
                content.appendChild(preview);
                item.appendChild(content);
                
                item.addEventListener('click', () => selectChatSession(session.chat_session));
                
                listContainer.appendChild(item);
        }

        async function selectChatSession(sessionName) {
            // Update active state
            const items = document.querySelectorAll('.sms-chat-session-item');
            items.forEach(item => {
                if (item.dataset.session === sessionName) {
                    item.classList.add('active');
                } else {
                    item.classList.remove('active');
                }
            });

            currentSession = sessionName;
            
            // Find the selected session to get its message_type
            const selectedSession = chatSessions.find(s => s.chat_session === sessionName);
            const messageType = selectedSession?.message_type || 'sms';
            
            const titleElement = document.getElementById('sms-conversation-title-text');
            const typeIconElement = document.getElementById('sms-conversation-type-icon');
            const deleteBtn = document.getElementById('sms-delete-conversation-btn');
            const askAIBtn = document.getElementById('sms-ask-ai-btn');
            
            if (titleElement) {
                titleElement.textContent = sessionName || 'Unknown Conversation';
            }
            
            // Update conversation type icon
            if (typeIconElement) {
                typeIconElement.innerHTML = ''; // Clear previous icon
                typeIconElement.style.display = sessionName ? 'inline-block' : 'none';
                
                if (sessionName) {
                    const icon = document.createElement('i');
                    if (messageType === 'imessage') {
                        icon.className = 'fab fa-apple';
                        icon.title = 'iMessage';
                        icon.style.color = '#007AFF';
                    } else if (messageType === 'sms') {
                        icon.className = 'fas fa-comment';
                        icon.title = 'SMS';
                        icon.style.color = '#34C759';
                    } else if (messageType === 'whatsapp') {
                        icon.className = 'fab fa-whatsapp';
                        icon.title = 'WhatsApp';
                        icon.style.color = '#25D366';
                    } else if (messageType === 'facebook') {
                        icon.className = 'fab fa-facebook-messenger';
                        icon.title = 'Facebook Messenger';
                        icon.style.color = '#0084FF';
                    } else if (messageType === 'instagram') {
                        icon.className = 'fab fa-instagram';
                        icon.title = 'Instagram';
                        icon.style.color = '#E4405F';
                    } else if (messageType === 'mixed') {
                        icon.className = 'fas fa-comments';
                        icon.title = 'Mixed (SMS, iMessage, WhatsApp, Facebook Messenger & Instagram)';
                        icon.style.color = '#FF9500';
                    }
                    typeIconElement.appendChild(icon);
                }
            }
            
            if (deleteBtn) {
                deleteBtn.style.display = sessionName ? 'block' : 'none';
            }
            
            if (askAIBtn) {
                askAIBtn.style.display = sessionName ? 'block' : 'none';
            }

            const messagesContainer = document.getElementById('sms-conversation-messages');
            const instructionsElement = document.getElementById('sms-conversation-instructions');
            
            if (instructionsElement) {
                instructionsElement.style.display = 'none';
            }

            if (!messagesContainer) return;

            messagesContainer.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666;">Loading messages...</div>';

            try {
                const encodedSession = encodeURIComponent(sessionName);
                const response = await fetch(`/imessages/conversation/${encodedSession}`);
                if (!response.ok) {
                    throw new Error('Failed to load messages');
                }
                const data = await response.json();
                displayMessages(data.messages || []);
            } catch (error) {
                console.error('Error loading messages:', error);
                messagesContainer.innerHTML = '<div style="text-align: center; padding: 2rem; color: #dc3545;">Error loading messages</div>';
            }
        }

        function displayMessages(messages) {
            const messagesContainer = document.getElementById('sms-conversation-messages');
            if (!messagesContainer) return;

            messagesContainer.innerHTML = '';

            messages.forEach((message, index) => {
                const messageDiv = document.createElement('div');
                const isIncoming = message.type === 'Incoming';
                messageDiv.className = `sms-message ${isIncoming ? 'incoming' : 'outgoing'}`;
                
                // Add ID to message div for scrolling to specific messages
                if (message.id) {
                    messageDiv.id = `message-${message.id}`;
                }
                
                // Add spacing between message groups
                if (index > 0) {
                    const prevMessage = messages[index - 1];
                    const timeDiff = new Date(message.message_date) - new Date(prevMessage.message_date);
                    const minutesDiff = timeDiff / (1000 * 60);
                    if (minutesDiff > 5 || prevMessage.type !== message.type) {
                        messageDiv.style.marginTop = '10px';
                    }
                }

                const bubble = document.createElement('div');
                bubble.className = 'sms-message-bubble';

                // Header with sender (for incoming) and date
                if (isIncoming) {
                    const header = document.createElement('div');
                    header.className = 'sms-message-header';
                    
                    // Service type icon and sender name
                    const leftContainer = document.createElement('div');
                    leftContainer.style.display = 'flex';
                    leftContainer.style.alignItems = 'center';
                    leftContainer.style.gap = '6px';
                    
                    // Service type icon
                    const serviceIcon = document.createElement('i');
                    serviceIcon.className = 'sms-message-service-icon';
                    serviceIcon.style.fontSize = '11px';
                    
                    const service = message.service || '';
                    if (service.toLowerCase().includes('imessage')) {
                        serviceIcon.className = 'fab fa-apple sms-message-service-icon';
                        serviceIcon.style.color = '#007AFF';
                        serviceIcon.title = 'iMessage';
                    } else if (service.toLowerCase().includes('sms')) {
                    serviceIcon.className = 'fas fa-comment sms-message-service-icon';
                    serviceIcon.style.color = '#34C759';
                    serviceIcon.title = 'SMS';
                } else if (service === 'WhatsApp') {
                    serviceIcon.className = 'fab fa-whatsapp sms-message-service-icon';
                    serviceIcon.style.color = '#25D366';
                    serviceIcon.title = 'WhatsApp';
                } else if (service === 'Facebook Messenger') {
                    serviceIcon.className = 'fab fa-facebook-messenger sms-message-service-icon';
                    serviceIcon.style.color = '#0084FF';
                    serviceIcon.title = 'Facebook Messenger';
                } else if (service === 'Instagram') {
                    serviceIcon.className = 'fab fa-instagram sms-message-service-icon';
                    serviceIcon.style.color = '#E4405F';
                    serviceIcon.title = 'Instagram';
                } else {
                    // Default icon if service type is unknown
                    serviceIcon.className = 'fas fa-comment sms-message-service-icon';
                    serviceIcon.style.color = '#666';
                    serviceIcon.title = service || 'Message';
                }
                
                const senderSpan = document.createElement('span');
                senderSpan.className = 'sms-message-sender';
                senderSpan.textContent = message.sender_name || message.sender_id || (isIncoming ? 'Incoming' : 'Outgoing');
                
                leftContainer.appendChild(serviceIcon);
                leftContainer.appendChild(senderSpan);
                
                const dateSpan = document.createElement('span');
                dateSpan.className = 'sms-message-date';
                dateSpan.textContent = formatAustralianDate(message.message_date);
                
                    header.appendChild(leftContainer);
                    messageDiv.appendChild(header);
                }

                // Message text
                if (message.text) {
                    const textDiv = document.createElement('div');
                    textDiv.className = 'sms-message-text';
                    textDiv.textContent = message.text;
                    bubble.appendChild(textDiv);
                }

                // Attachment
                if (message.has_attachment && message.attachment_filename) {
                    const attachmentDiv = document.createElement('div');
                    attachmentDiv.className = 'sms-message-attachment';
                    attachmentDiv.style.marginTop = '4px';
                    attachmentDiv.style.borderRadius = '7.5px';
                    attachmentDiv.style.overflow = 'hidden';
                    attachmentDiv.style.cursor = 'pointer';
                    
                    // Check attachment type to display appropriately
                    const contentType = message.attachment_type || '';
                    const isAudio = contentType.startsWith('audio/');
                    const isVideo = contentType.startsWith('video/');
                    const isImage = contentType.startsWith('image/');
                    
                    // Helper function to create fallback display
                    const createFallbackDisplay = (iconClass, iconText) => {
                        attachmentDiv.innerHTML = `<div style="padding: 12px; background-color: rgba(0,0,0,0.05); border-radius: 7.5px; font-size: 13px; color: #667781; display: flex; align-items: center; gap: 8px;"><i class="${iconClass}" style="font-size: 16px;"></i><span>${message.attachment_filename}</span></div>`;
                    };
                    
                    if (isAudio) {
                        // Display audio player inline in the message bubble
                        // Remove overflow hidden for audio to show controls properly
                        attachmentDiv.style.overflow = 'visible';
                        attachmentDiv.style.maxWidth = '300px';
                        
                        const audio = document.createElement('audio');
                        audio.src = `/imessages/${message.id}/attachment`;
                        audio.controls = true;
                        audio.preload = 'metadata'; // Load metadata but not the full audio until play
                        audio.style.width = '100%';
                        audio.style.minWidth = '250px';
                        audio.style.minHeight = '40px';
                        audio.style.height = 'auto';
                        audio.style.display = 'block';
                        audio.style.outline = 'none';
                        audio.style.verticalAlign = 'middle';
                        attachmentDiv.appendChild(audio);
                        // Remove cursor pointer since audio controls handle interaction
                        attachmentDiv.style.cursor = 'default';
                    } else if (isVideo) {
                        // Display video attachment with icon
                        createFallbackDisplay('fas fa-video', 'Video');
                    } else {
                        // For images or unknown types, try to display as image first
                        // This handles cases where attachment_type might not be set
                        const img = document.createElement('img');
                        img.loading = 'lazy'; // Native lazy loading - MUST be set before src
                        img.src = `/imessages/${message.id}/attachment`;
                        img.alt = message.attachment_filename;
                        img.style.maxWidth = '300px';
                        img.style.maxHeight = '300px';
                        img.style.objectFit = 'cover';
                        img.style.display = 'block';
                        img.style.cursor = 'pointer';
                        
                        img.onerror = function() {
                            // If image fails to load, show filename with appropriate icon
                            if (isImage) {
                                // Known image that failed to load
                                createFallbackDisplay('fas fa-image', 'Image');
                            } else {
                                // Unknown file type
                                createFallbackDisplay('fas fa-file', 'File');
                            }
                        };
                        
                        attachmentDiv.appendChild(img);
                    }
                    
                    // Attach click handler to the attachment div (skip audio since it has inline controls)
                    if (!isAudio) {
                        attachmentDiv.addEventListener('click', () => {
                            showFullAttachment(message.id, message.attachment_filename, message.attachment_type);
                        });
                    }
                    
                    bubble.appendChild(attachmentDiv);
                }

                // Timestamp stacked vertically next to bubble - show full date and time
                const dateTimeContainer = document.createElement('div');
                dateTimeContainer.style.display = 'flex';
                dateTimeContainer.style.flexDirection = 'column';
                dateTimeContainer.style.alignItems = 'flex-start';
                dateTimeContainer.style.marginLeft = '6px';
                dateTimeContainer.style.paddingBottom = '2px';
                dateTimeContainer.style.justifyContent = 'flex-end';
                
                // Show full Australian date format: DD/MM/YYYY HH:MM:SS
                const fullDateStr = formatAustralianDate(message.message_date);
                const dateTimeParts = fullDateStr.split(' ');
                
                // Date part
                const dateSpan = document.createElement('span');
                dateSpan.className = 'sms-message-date';
                dateSpan.style.fontSize = '11px';
                dateSpan.style.color = '#667781';
                dateSpan.style.whiteSpace = 'nowrap';
                dateSpan.style.lineHeight = '1.2';
                dateSpan.textContent = dateTimeParts[0] || ''; // Date part
                
                // Time part
                const timeSpan = document.createElement('span');
                timeSpan.className = 'sms-message-time';
                timeSpan.style.fontSize = '11px';
                timeSpan.style.color = '#667781';
                timeSpan.style.whiteSpace = 'nowrap';
                timeSpan.style.lineHeight = '1.2';
                timeSpan.textContent = dateTimeParts.length > 1 ? dateTimeParts.slice(1).join(' ') : ''; // Time part(s)
                
                dateTimeContainer.appendChild(dateSpan);
                dateTimeContainer.appendChild(timeSpan);
                
                // Wrap bubble and timestamp together
                const contentWrapper = document.createElement('div');
                contentWrapper.style.display = 'flex';
                contentWrapper.style.alignItems = 'flex-end';
                contentWrapper.style.gap = '6px';
                contentWrapper.appendChild(bubble);
                contentWrapper.appendChild(dateTimeContainer);
                
                messageDiv.appendChild(contentWrapper);
                messagesContainer.appendChild(messageDiv);
            });

            // Scroll to bottom
            messagesContainer.scrollTop = messagesContainer.scrollHeight;
        }

        function showFullAttachment(messageId, filename, contentType) {
            // Use existing single image modal
            const modal = document.getElementById('single-image-modal');
            const modalImg = document.getElementById('single-image-modal-img');
            const modalVideo = document.getElementById('single-image-modal-video');
            const modalAudio = document.getElementById('single-image-modal-audio');
            const modalPdf = document.getElementById('single-image-modal-pdf');
            const modalDetails = document.getElementById('single-image-details');

            if (!modal) return;

            // Hide all media elements
            modalImg.style.display = 'none';
            modalVideo.style.display = 'none';
            modalAudio.style.display = 'none';
            modalPdf.style.display = 'none';

            const attachmentUrl = `/imessages/${messageId}/attachment`;
            const isImage = contentType && contentType.startsWith('image/');
            const isVideo = contentType && contentType.startsWith('video/');
            const isAudio = contentType && contentType.startsWith('audio/');
            const isPdf = contentType === 'application/pdf';

            if (modalDetails) {
                modalDetails.textContent = filename || 'Attachment';
            }

            if (isImage) {
                modalImg.src = attachmentUrl;
                modalImg.style.display = 'block';
            } else if (isVideo) {
                modalVideo.src = attachmentUrl;
                modalVideo.style.display = 'block';
            } else if (isAudio) {
                modalAudio.src = attachmentUrl;
                modalAudio.style.display = 'block';
                // Auto-play the audio
                modalAudio.play().catch(error => {
                    console.warn('Auto-play prevented by browser:', error);
                    // Audio will still be available for manual play via controls
                });
            } else if (isPdf) {
                modalPdf.src = attachmentUrl;
                modalPdf.style.display = 'block';
            } else {
                // For other file types, try to show as image first
                modalImg.src = attachmentUrl;
                modalImg.style.display = 'block';
                modalImg.onerror = function() {
                    // If it fails, show download link
                    modalImg.style.display = 'none';
                    if (modalDetails) {
                        modalDetails.innerHTML = `<div style="padding: 20px; text-align: center;">
                            <p>${filename || 'Attachment'}</p>
                            <a href="${attachmentUrl}" download style="color: #4a90e2; text-decoration: underline;">Download</a>
                        </div>`;
                    }
                };
            }

            modal.style.display = 'flex';
        }

        function searchChatSessions(query) {
            const searchTerm = query.toLowerCase().trim();
            if (!searchTerm) {
                filteredSessions = [...chatSessions];
            } else {
                filteredSessions = chatSessions.filter(session => 
                    session.chat_session && session.chat_session.toLowerCase().includes(searchTerm)
                );
            }
            renderChatSessions();
        }

        async function deleteConversation() {
            if (!currentSession) return;

            const confirmed = await AppDialogs.showAppConfirm(
                'Delete conversation',
                `Are you sure you want to delete the conversation "${currentSession}"?\n\nThis action cannot be undone.`,
                { danger: true }
            );
            if (!confirmed) {
                return;
            }

            try {
                const encodedSession = encodeURIComponent(currentSession);
                const response = await fetch(`/imessages/conversation/${encodedSession}`, {
                    method: 'DELETE'
                });

                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.detail || 'Failed to delete conversation');
                }

                const result = await response.json();
                
                // Clear the conversation view
                currentSession = null;
                const titleElement = document.getElementById('sms-conversation-title');
                const deleteBtn = document.getElementById('sms-delete-conversation-btn');
                const messagesContainer = document.getElementById('sms-conversation-messages');
                const instructionsElement = document.getElementById('sms-conversation-instructions');
                
                if (titleElement) {
                    titleElement.textContent = 'Select a conversation';
                }
                if (deleteBtn) {
                    deleteBtn.style.display = 'none';
                }
                if (messagesContainer) {
                    messagesContainer.innerHTML = '';
                }
                if (instructionsElement) {
                    instructionsElement.style.display = 'block';
                }

                // Remove active state from all items
                const items = document.querySelectorAll('.sms-chat-session-item');
                items.forEach(item => item.classList.remove('active'));

                // Reload chat sessions list
                await loadChatSessions();
                
                await AppDialogs.showAppAlert('Success', `Successfully deleted ${result.deleted_count} message(s) from the conversation.`);
            } catch (error) {
                console.error('Error deleting conversation:', error);
                await AppDialogs.showAppAlert('Error', `Error deleting conversation: ${error.message}`);
            }
        }

        function init() {
            const searchInput = document.getElementById('sms-chat-search');
            if (searchInput) {
                searchInput.addEventListener('input', (e) => {
                    searchChatSessions(e.target.value);
                });
            }

            // Add event listeners for message type filter checkboxes
            const filterCheckboxes = {
                'filter-imessage': 'imessage',
                'filter-sms': 'sms',
                'filter-whatsapp': 'whatsapp',
                'filter-facebook': 'facebook',
                'filter-instagram': 'instagram',
                'filter-mixed': 'mixed'
            };

            // Initialize checkboxes and sync with messageTypeFilters
            Object.keys(filterCheckboxes).forEach(checkboxId => {
                const checkbox = document.getElementById(checkboxId);
                if (checkbox) {
                    const filterKey = filterCheckboxes[checkboxId];
                    const label = checkbox.closest('label');
                    
                    // Sync filter state from checkbox's actual checked state
                    // Read the current checkbox state (which may be set by HTML checked attribute)
                    const isChecked = checkbox.checked;
                    messageTypeFilters[filterKey] = isChecked;
                    
                    // Update label styling based on checked state
                    if (label) {
                        const icon = label.querySelector('i');
                        const textSpan = label.querySelector('span');
                        
                        if (isChecked) {
                            label.style.backgroundColor = '#d1e7dd';
                            label.style.borderColor = '#0f5132';
                            if (icon) icon.style.color = '#0f5132';
                            if (textSpan) {
                                textSpan.style.color = '#0f5132';
                                textSpan.style.fontWeight = '600';
                            }
                        } else {
                            label.style.backgroundColor = '#f0f2f5';
                            label.style.borderColor = '#f0f2f5';
                            // Reset icon colors to their original
                            if (icon) {
                                const iconClass = icon.className;
                                if (iconClass.includes('fa-apple')) icon.style.color = '#007AFF';
                                else if (iconClass.includes('fa-comment') && !iconClass.includes('fa-comments')) icon.style.color = '#34C759';
                                else if (iconClass.includes('fa-whatsapp')) icon.style.color = '#25D366';
                                else if (iconClass.includes('fa-facebook-messenger')) icon.style.color = '#0084FF';
                                else if (iconClass.includes('fa-instagram')) icon.style.color = '#E4405F';
                                else if (iconClass.includes('fa-comments')) icon.style.color = '#FF9500';
                                else icon.style.color = '#54656f';
                            }
                            if (textSpan) {
                                textSpan.style.color = '#54656f';
                                textSpan.style.fontWeight = 'normal';
                            }
                        }
                    }
                    
                    checkbox.addEventListener('change', (e) => {
                        const newValue = Boolean(e.target.checked);
                        messageTypeFilters[filterKey] = newValue;
                        
                        // Update label styling
                        if (label) {
                            const icon = label.querySelector('i');
                            const textSpan = label.querySelector('span');
                            
                            if (newValue) {
                                label.style.backgroundColor = '#d1e7dd';
                                label.style.borderColor = '#0f5132';
                                if (icon) icon.style.color = '#0f5132';
                                if (textSpan) {
                                    textSpan.style.color = '#0f5132';
                                    textSpan.style.fontWeight = '600';
                                }
                            } else {
                                label.style.backgroundColor = '#f0f2f5';
                                label.style.borderColor = '#f0f2f5';
                                // Reset icon colors to their original
                                if (icon) {
                                    const iconClass = icon.className;
                                    if (iconClass.includes('fa-apple')) icon.style.color = '#007AFF';
                                    else if (iconClass.includes('fa-comment') && !iconClass.includes('fa-comments')) icon.style.color = '#34C759';
                                    else if (iconClass.includes('fa-whatsapp')) icon.style.color = '#25D366';
                                    else if (iconClass.includes('fa-facebook-messenger')) icon.style.color = '#0084FF';
                                    else if (iconClass.includes('fa-instagram')) icon.style.color = '#E4405F';
                                    else if (iconClass.includes('fa-comments')) icon.style.color = '#FF9500';
                                    else icon.style.color = '#54656f';
                                }
                                if (textSpan) {
                                    textSpan.style.color = '#54656f';
                                    textSpan.style.fontWeight = 'normal';
                                }
                            }
                        }
                        
                        renderChatSessions();
                    });
                } else {
                    console.warn(`Checkbox not found: ${checkboxId}`);
                }
            });
            
            // After initializing all checkboxes, ensure filter state is synced
            // and trigger a render if sessions are already loaded
            if (chatSessions.length > 0) {
                renderChatSessions();
            }

            const deleteBtn = document.getElementById('sms-delete-conversation-btn');
            if (deleteBtn) {
                deleteBtn.addEventListener('click', (e) => {
                    e.stopPropagation(); // Prevent any event bubbling
                    deleteConversation();
                });
            }

            // Ask AI button and modal handlers
            const askAIBtn = document.getElementById('sms-ask-ai-btn');
            const askAIModal = document.getElementById('sms-ask-ai-modal');
            const askAICloseBtn = document.getElementById('sms-ask-ai-close');
            const askAICancelBtn = document.getElementById('sms-ask-ai-cancel');
            const askAISubmitBtn = document.getElementById('sms-ask-ai-submit');
            const askAIRadioButtons = askAIModal ? askAIModal.querySelectorAll('input[name="sms-ask-ai-option"]') : [];
            const askAIOtherTextarea = document.getElementById('sms-ask-ai-other-text');
            const askAIOtherInput = document.getElementById('sms-ask-ai-other-input');

            if (askAIBtn && askAIModal) {
                askAIBtn.addEventListener('click', () => {
                    // Update conversation title in modal
                    const conversationTitleEl = document.getElementById('sms-ask-ai-conversation-title');
                    const conversationTitle = document.getElementById('sms-conversation-title');
                    if (conversationTitleEl && conversationTitle) {
                        conversationTitleEl.textContent = conversationTitle.textContent || 'Unknown Conversation';
                    }
                    askAIModal.style.display = 'flex';
                });
            }

            if (askAICloseBtn && askAIModal) {
                askAICloseBtn.addEventListener('click', () => {
                    askAIModal.style.display = 'none';
                });
            }

            if (askAICancelBtn && askAIModal) {
                askAICancelBtn.addEventListener('click', () => {
                    askAIModal.style.display = 'none';
                });
            }

            if (askAISubmitBtn) {
                askAISubmitBtn.addEventListener('click', async (e) => {
                    e.preventDefault();
                    e.stopPropagation();

                    const selectedOption = askAIModal
                        ? askAIModal.querySelector('input[name="sms-ask-ai-option"]:checked')?.value
                        : undefined;
                    const instruction = (askAIOtherInput && askAIOtherInput.value ? askAIOtherInput.value : '').trim();

                    const useCustomPrompt = selectedOption === 'other' || instruction.length > 0;

                    if (useCustomPrompt) {
                        if (!instruction) {
                            await AppDialogs.showAppAlert('Please enter your question or request in the text box.');
                            return;
                        }
                        if (!currentSession) {
                            await AppDialogs.showAppAlert('No conversation selected');
                            return;
                        }
                        if (!Modals.ConversationSummary || typeof Modals.ConversationSummary.openWithIMessagePrompt !== 'function') {
                            console.error('ConversationSummary.openWithIMessagePrompt is not available');
                            await AppDialogs.showAppAlert('Ask the AI is not ready. Please refresh the page.');
                            return;
                        }
                        if (askAIModal) {
                            askAIModal.style.display = 'none';
                        }
                        try {
                            Modals.ConversationSummary.openWithIMessagePrompt(currentSession, instruction);
                        } catch (error) {
                            console.error('Error opening Ask the AI prompt:', error);
                            await AppDialogs.showAppAlert('Failed to start the AI request. Please try again.');
                        }
                        return;
                    }

                    if (selectedOption === 'summarise') {
                        if (!currentSession) {
                            await AppDialogs.showAppAlert('No conversation selected');
                            return;
                        }
                        if (askAIModal) {
                            askAIModal.style.display = 'none';
                        }
                        try {
                            Modals.ConversationSummary.open(currentSession);
                        } catch (error) {
                            console.error('Error opening conversation summary:', error);
                            await AppDialogs.showAppAlert('Failed to start conversation summarization. Please try again.');
                        }
                        return;
                    }

                    if (askAIModal) {
                        askAIModal.style.display = 'none';
                    }
                });
            }


            // Close modal when clicking outside
            if (askAIModal) {
                askAIModal.addEventListener('click', (e) => {
                    if (e.target === askAIModal) {
                        askAIModal.style.display = 'none';
                    }
                });
            }

            const closeBtn = document.getElementById('close-sms-messages-modal');
            if (closeBtn) {
                closeBtn.addEventListener('click', () => {
                    const modal = document.getElementById('sms-messages-modal');
                    if (modal) {
                        modal.style.display = 'none';
                    }
                });
            }

            const modal = document.getElementById('sms-messages-modal');
            if (modal) {
                modal.addEventListener('click', (e) => {
                    if (e.target === modal) {
                        modal.style.display = 'none';
                    }
                });
            }
        }

        async function openWithFilter(contactName) {
            await open();
            const searchInput = document.getElementById('sms-chat-search');
            if (searchInput && contactName) {
                searchInput.value = contactName;
                searchChatSessions(contactName);
                if (filteredSessions.length > 0) {
                    const exactMatch = filteredSessions.find(s => s.chat_session === contactName);
                    const toSelect = exactMatch || filteredSessions[0];
                    await selectChatSession(toSelect.chat_session);
                }
            }
        }

        async function open() {
            const modal = document.getElementById('sms-messages-modal');
            if (modal) {
                modal.style.display = 'flex';
                // Re-initialize filters from checkboxes when modal opens
                // This ensures filter state matches checkbox state
                const filterCheckboxes = {
                    'filter-imessage': 'imessage',
                    'filter-sms': 'sms',
                    'filter-whatsapp': 'whatsapp',
                    'filter-facebook': 'facebook',
                    'filter-instagram': 'instagram',
                    'filter-mixed': 'mixed'
                };
                
                Object.keys(filterCheckboxes).forEach(checkboxId => {
                    const checkbox = document.getElementById(checkboxId);
                    if (checkbox) {
                        const filterKey = filterCheckboxes[checkboxId];
                        const label = checkbox.closest('label');
                        const isChecked = Boolean(checkbox.checked);
                        messageTypeFilters[filterKey] = isChecked;
                        
                        // Update label styling
                        if (label) {
                            const icon = label.querySelector('i');
                            const textSpan = label.querySelector('span');
                            
                            if (isChecked) {
                                label.style.backgroundColor = '#d1e7dd';
                                label.style.borderColor = '#0f5132';
                                if (icon) icon.style.color = '#0f5132';
                                if (textSpan) {
                                    textSpan.style.color = '#0f5132';
                                    textSpan.style.fontWeight = '600';
                                }
                            } else {
                                label.style.backgroundColor = '#f0f2f5';
                                label.style.borderColor = '#f0f2f5';
                                // Reset icon colors to their original
                                if (icon) {
                                    const iconClass = icon.className;
                                    if (iconClass.includes('fa-apple')) icon.style.color = '#007AFF';
                                    else if (iconClass.includes('fa-comment') && !iconClass.includes('fa-comments')) icon.style.color = '#34C759';
                                    else if (iconClass.includes('fa-whatsapp')) icon.style.color = '#25D366';
                                    else if (iconClass.includes('fa-facebook-messenger')) icon.style.color = '#0084FF';
                                    else if (iconClass.includes('fa-instagram')) icon.style.color = '#E4405F';
                                    else if (iconClass.includes('fa-comments')) icon.style.color = '#FF9500';
                                    else icon.style.color = '#54656f';
                                }
                                if (textSpan) {
                                    textSpan.style.color = '#54656f';
                                    textSpan.style.fontWeight = 'normal';
                                }
                            }
                        }
                    }
                });
                
                await loadChatSessions()
            }
        }
        async function openAndSelectConversation(messageID){
            // Open the modal first
            open();
            
            try {
                // Retrieve the message metadata by messageID
                const messageResponse = await fetch(`/imessages/${messageID}/metadata`);
                if (!messageResponse.ok) {
                    throw new Error(`Failed to fetch message metadata: ${messageResponse.status}`);
                }
                
                const messageMetadata = await messageResponse.json();
                const chatSession = messageMetadata.chat_session;
                
                if (!chatSession) {
                    console.error('Message metadata does not contain chat_session');
                    await AppDialogs.showAppAlert('Unable to open conversation: Message has no chat session');
                    return;
                }
                
                // Load chat sessions if not already loaded
                if (chatSessions.length === 0) {
                    await loadChatSessions();
                }
                
                // Wait a bit for the DOM to update after loading sessions
                await new Promise(resolve => setTimeout(resolve, 100));
                
                // Find the conversation by chat_session name
                const conversation = chatSessions.find(s => s.chat_session === chatSession);
                
                if (conversation) {
                    // Select the conversation (this loads and displays messages)
                    await selectChatSession(chatSession);
                    
                    // Wait for messages to be rendered in the DOM
                    await new Promise(resolve => setTimeout(resolve, 200));
                    
                    // Scroll to the specific message
                    const message = document.getElementById('message-'+messageID);
                    if (message) {
                        message.scrollIntoView({ behavior: 'smooth', block: 'center' });
                    } else {
                        console.warn(`Message with ID ${messageID} not found in DOM`);
                    }
                } else {
                    console.warn(`Conversation with chat_session "${chatSession}" not found`);
                    // Still render sessions even if the specific one isn't found
                    renderChatSessions();
                    await AppDialogs.showAppAlert(`Conversation "${chatSession}" not found in the list`);
                }
            } catch (error) {
                console.error('Error opening conversation:', error);
                await AppDialogs.showAppAlert('Failed to open conversation. Please try again.');
                // Still render sessions on error
                if (chatSessions.length === 0) {
                    await loadChatSessions();
                } else {
                    renderChatSessions();
                }
            }
        }
        

        function close() {
            const modal = document.getElementById('sms-messages-modal');
            if (modal) {
                modal.style.display = 'none';
            }
        }

        return { init, open, close, openAndSelectConversation, openWithFilter };
})();


