// app.js is Receiptd's sitewide progressive-enhancement script, loaded
// unconditionally by base.tmpl on every page (unlike dashboard.js/
// builder.js, which are per-page). Every DOM change here uses
// classList/attributes, never element.style — the CSP (router.go) sets
// style-src 'self', so an inline style write would silently no-op.
(function () {
  "use strict";

  var THEME_STORAGE_KEY = "receiptd-theme";

  function highlightActiveNav() {
    var path = window.location.pathname;
    document.querySelectorAll("nav a").forEach(function (a) {
      if (a.getAttribute("href") === path) {
        a.classList.add("active");
      }
    });
  }

  function confirmDestructiveSubmit(event) {
    var submitter = event.submitter;
    var message = submitter && submitter.dataset ? submitter.dataset.confirm : null;
    if (message && !window.confirm(message)) {
      event.preventDefault();
      return false;
    }
    return true;
  }

  var PENDING_LABEL = "Please wait…";

  function setButtonLabel(btn, label) {
    if (btn.tagName === "INPUT") {
      btn.value = label;
    } else {
      btn.textContent = label;
    }
  }

  // Every form here does a full page navigation on submit, and these
  // buttons carry no name/value pair — disabling after dispatch can't drop
  // submitted data.
  function setSubmitPending(form) {
    var buttons = form.querySelectorAll('button[type="submit"], input[type="submit"]');
    buttons.forEach(function (btn) {
      btn.dataset.idleLabel = btn.tagName === "INPUT" ? btn.value : btn.textContent;
      btn.disabled = true;
      setButtonLabel(btn, PENDING_LABEL);
    });
  }

  // The back/forward cache restores a page exactly as it was left, submit
  // buttons still disabled — unusable until a manual reload.
  function clearSubmitPending() {
    document.querySelectorAll("[data-idle-label]").forEach(function (btn) {
      btn.disabled = false;
      setButtonLabel(btn, btn.dataset.idleLabel);
      delete btn.dataset.idleLabel;
    });
  }

  function onSubmit(event) {
    // builder.js's form-level listener runs first and may have blocked it.
    if (event.defaultPrevented) {
      return;
    }
    if (!confirmDestructiveSubmit(event)) {
      return;
    }
    setSubmitPending(event.target);
  }

  function initDropzones() {
    document.querySelectorAll(".dropzone").forEach(function (zone) {
      var input = zone.querySelector('input[type="file"]');
      if (!input) {
        return;
      }

      ["dragenter", "dragover"].forEach(function (type) {
        zone.addEventListener(type, function (e) {
          e.preventDefault();
          zone.classList.add("dragover");
        });
      });

      ["dragleave", "drop"].forEach(function (type) {
        zone.addEventListener(type, function (e) {
          e.preventDefault();
          zone.classList.remove("dragover");
        });
      });

      zone.addEventListener("drop", function (e) {
        var files = e.dataTransfer && e.dataTransfer.files;
        if (files && files.length) {
          input.files = files;
        }
      });
    });
  }

  // localStorage can throw (private browsing, disabled storage); theming
  // is a nicety, so a failure here just means it doesn't persist.
  function readStoredTheme() {
    try {
      return window.localStorage.getItem(THEME_STORAGE_KEY);
    } catch (e) {
      return null;
    }
  }

  function writeStoredTheme(value) {
    try {
      if (value === null) {
        window.localStorage.removeItem(THEME_STORAGE_KEY);
      } else {
        window.localStorage.setItem(THEME_STORAGE_KEY, value);
      }
    } catch (e) {
      // No-op: falls back to not persisting.
    }
  }

  // "system" (no stored preference) is represented as no data-theme
  // attribute at all — style.css's prefers-color-scheme block then decides
  // light/dark, so the CSS only handles the three real presets.
  function applyStoredTheme() {
    var stored = readStoredTheme();
    if (stored) {
      document.documentElement.setAttribute("data-theme", stored);
    }
  }

  function initThemePicker() {
    var picker = document.getElementById("theme-picker");
    if (!picker) {
      return;
    }
    picker.value = readStoredTheme() || "system";

    picker.addEventListener("change", function () {
      if (picker.value === "system") {
        document.documentElement.removeAttribute("data-theme");
        writeStoredTheme(null);
      } else {
        document.documentElement.setAttribute("data-theme", picker.value);
        writeStoredTheme(picker.value);
      }
    });
  }

  // Not deferred to DOMContentLoaded like the rest: waiting lets the
  // default theme paint first and flash on every navigation.
  applyStoredTheme();

  document.addEventListener("DOMContentLoaded", function () {
    highlightActiveNav();
    initDropzones();
    initThemePicker();
  });

  document.addEventListener("submit", onSubmit);
  window.addEventListener("pageshow", clearSubmitPending);
})();
