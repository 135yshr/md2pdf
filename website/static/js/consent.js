// Google Analytics Consent Mode v2
// Must be loaded as a blocking script BEFORE the GA tag.
(function () {
  'use strict';

  var STORAGE_KEY = 'md2pdf_consent';

  // Consent parameter sets
  var GRANTED = {
    ad_storage: 'granted',
    ad_user_data: 'granted',
    ad_personalization: 'granted',
    analytics_storage: 'granted'
  };

  var DENIED = {
    ad_storage: 'denied',
    ad_user_data: 'denied',
    ad_personalization: 'denied',
    analytics_storage: 'denied'
  };

  // Ensure dataLayer and gtag exist
  window.dataLayer = window.dataLayer || [];
  function gtag() { dataLayer.push(arguments); }
  window.gtag = gtag;

  // Set default consent state — must fire before gtag('config', ...)
  gtag('consent', 'default', {
    ad_storage: 'denied',
    ad_user_data: 'denied',
    ad_personalization: 'denied',
    analytics_storage: 'denied',
    wait_for_update: 500
  });

  // DNT check — if enabled, stay denied and never show banner
  if (navigator.doNotTrack === '1' || window.doNotTrack === '1') {
    return;
  }

  // Check stored preference
  var stored = null;
  try { stored = localStorage.getItem(STORAGE_KEY); } catch (e) { /* ignore */ }

  if (stored === 'granted') {
    gtag('consent', 'update', GRANTED);
    gtag('event', 'page_view');
    return;
  }
  if (stored === 'denied') {
    return;
  }

  // No stored preference — show banner when DOM is ready
  function showBanner() {
    var banner = document.getElementById('md2pdf-consent-banner');
    if (!banner) return;
    banner.hidden = false;

    var acceptBtn = document.getElementById('md2pdf-consent-accept');
    var rejectBtn = document.getElementById('md2pdf-consent-reject');
    if (!acceptBtn || !rejectBtn) return;

    acceptBtn.addEventListener('click', function () {
      try { localStorage.setItem(STORAGE_KEY, 'granted'); } catch (e) { /* ignore */ }
      gtag('consent', 'update', GRANTED);
      gtag('event', 'page_view');
      banner.hidden = true;
    });

    rejectBtn.addEventListener('click', function () {
      try { localStorage.setItem(STORAGE_KEY, 'denied'); } catch (e) { /* ignore */ }
      gtag('consent', 'update', DENIED);
      banner.hidden = true;
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', showBanner);
  } else {
    showBanner();
  }
})();
