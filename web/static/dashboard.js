// dashboard.js polls GET /status on a timer and updates the dashboard's
// Printers/Queue cards in place, so an operator who leaves this tab open
// notices a printer going offline or a job finishing without reloading
// (docs/adr/0025-dashboard-updates-via-polling.md). This is the Web UI's
// only script — plain, dependency-free, and scoped to this one job per
// docs/adr/0022-webui-server-rendered-html-template.md: no framework, no
// other client-side behavior.
(function () {
  "use strict";

  // 5s: frequent enough that an operator watching the tab notices a
  // change quickly, infrequent enough to be negligible load for the
  // homelab/Raspberry-Pi scale docs/adr/0025 targets. Not part of that
  // ADR's decision itself — just this script's own implementation
  // detail, free to retune without revisiting the ADR.
  var POLL_INTERVAL_MS = 5000;

  function setText(id, text) {
    var el = document.getElementById(id);
    if (el) {
      el.textContent = text;
    }
  }

  function applyStatus(data) {
    setText("printers-online", data.printers_online + " / " + data.printer_count + " online");
    setText("printers-status-message", data.status_message);
    setText("queue-pending", "Pending: " + data.queue_pending);
    setText("queue-running", "Running: " + data.queue_running);
    setText("queue-done", "Done: " + data.queue_done);
    setText("queue-failed", "Failed: " + data.queue_failed);
    setText("queue-cancelled", "Cancelled: " + data.queue_cancelled);
    setText("queue-total", data.queue_total + " total");
  }

  function poll() {
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
      });
  }

  // Guards against running on a page that doesn't have these elements —
  // this script is only ever included by dashboard.tmpl, but the guard
  // keeps it a no-op rather than a console error if that ever changes.
  if (document.getElementById("printers-online")) {
    poll();
    setInterval(poll, POLL_INTERVAL_MS);
  }
})();
