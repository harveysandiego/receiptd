// dashboard.js polls GET /status and updates the dashboard's Printers/
// Queue cards in place, so an operator who leaves this tab open notices a
// printer going offline or a job finishing without reloading
// (docs/adr/0025-dashboard-updates-via-polling.md). Like app.js it is
// plain and dependency-free, and scoped to this one job per
// docs/adr/0022-webui-server-rendered-html-template.md: no framework.
(function () {
  "use strict";

  // 5s: frequent enough that an operator watching the tab notices a
  // change quickly, infrequent enough to be negligible load for the
  // homelab/Raspberry-Pi scale docs/adr/0025 targets. Not part of that
  // ADR's decision itself — just this script's own implementation
  // detail, free to retune without revisiting the ADR.
  var POLL_INTERVAL_MS = 5000;

  // timer is non-null exactly when a poll is scheduled to run next (via
  // setTimeout, below) — null means polling is currently paused because
  // the tab is hidden. inFlight is true for the (usually brief) window
  // between starting a fetch and it settling, when timer is also null
  // but for a different reason — onVisibilityChange must check both, or
  // a hidden/visible flip during an in-flight request would start a
  // second, overlapping one.
  var timer = null;
  var inFlight = false;

  function setText(id, text) {
    var el = document.getElementById(id);
    if (el) {
      el.textContent = text;
    }
  }

  // Flashes the element when its text actually changes between polls, so a
  // value ticking over (e.g. queue-failed) is noticeable without staring
  // at the tab. The "flash" class is removed on the animation's own
  // "animationend" event (below), not a timeout.
  function setTextWithFlash(id, text) {
    var el = document.getElementById(id);
    if (!el || el.textContent === text) {
      return;
    }
    el.textContent = text;
    el.classList.add("flash");
  }

  function applyStatus(data) {
    setTextWithFlash("printers-online", data.printers_online + " / " + data.printer_count + " online");
    setText("printers-status-message", data.status_message);
    setTextWithFlash("queue-pending", "Pending: " + data.queue_pending);
    setTextWithFlash("queue-running", "Running: " + data.queue_running);
    setTextWithFlash("queue-done", "Done: " + data.queue_done);
    setTextWithFlash("queue-failed", "Failed: " + data.queue_failed);
    setTextWithFlash("queue-cancelled", "Cancelled: " + data.queue_cancelled);
    setTextWithFlash("queue-total", data.queue_total + " total");
  }

  // poll schedules its own successor only once its fetch fully settles
  // (via .finally below), rather than running on a fixed setInterval —
  // that's what guarantees at most one request in flight at a time: the
  // next poll can't start until this one has finished, no matter how
  // long it takes.
  function poll() {
    if (document.hidden) {
      // Paused: no fetch, and nothing scheduled — onVisibilityChange
      // resumes this once the tab is visible again.
      timer = null;
      return;
    }

    inFlight = true;

    // credentials: "same-origin" is already fetch's default for a
    // same-origin request like this one — stated explicitly so a future
    // reader doesn't have to recall that default to know this poll
    // carries the browser's cached Basic Auth credential the same way
    // the page's own initial load did (docs/adr/0023-webui-authentication-reuses-shared-token.md).
    fetch("/status", {
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    })
      .then(function (res) {
        if (!res.ok) {
          throw new Error("status " + res.status);
        }
        return res.json();
      })
      .then(applyStatus)
      .catch(function () {
        // Between polls the dashboard already shows the last-fetched
        // state (docs/adr/0025) — a failed poll just retries next
        // interval rather than replacing real data with an error.
      })
      .finally(function () {
        inFlight = false;
        timer = setTimeout(poll, POLL_INTERVAL_MS);
      });
  }

  // Resumes immediately on becoming visible again, rather than waiting
  // out whatever's left of a stale interval — but only when poll() isn't
  // already paused-and-waiting or already running (see timer/inFlight
  // above).
  function onVisibilityChange() {
    if (!document.hidden && timer === null && !inFlight) {
      poll();
    }
  }

  function onFlashAnimationEnd(e) {
    if (e.animationName === "flash-bg") {
      e.target.classList.remove("flash");
    }
  }

  // Guards against running on a page that doesn't have these elements —
  // this script is only ever included by dashboard.tmpl, but the guard
  // keeps it a no-op rather than a console error if that ever changes.
  if (document.getElementById("printers-online")) {
    document.addEventListener("visibilitychange", onVisibilityChange);
    document.addEventListener("animationend", onFlashAnimationEnd);
    poll();
  }
})();
