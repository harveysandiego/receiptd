// builder.js replaces raw-JSON editing on the Print/Preview pages with a
// form-based editor for a receipt.Receipt, for every element type
// internal/receipt/*.go defines. It only ever writes into the existing
// <textarea name="receipt">, which is still the field that actually gets
// submitted — print.go/preview.go are unmodified and unaware this script
// exists (docs/adr/0022: progressive enhancement, no framework, no build
// step). Every DOM change here uses classList/attributes, never
// element.style (see router.go's style-src 'self' CSP).
(function () {
  "use strict";

  var ELEMENT_TYPES = [
    { type: "text", label: "Text" },
    { type: "heading", label: "Heading" },
    { type: "divider", label: "Divider" },
    { type: "spacer", label: "Spacer" },
    { type: "feed", label: "Feed" },
    { type: "cut", label: "Cut" },
    { type: "qrcode", label: "QR code" },
    { type: "barcode", label: "Barcode" },
    { type: "asset", label: "Asset" },
    { type: "image", label: "Image" },
    { type: "table", label: "Table" },
    { type: "list", label: "List" },
    { type: "columns", label: "Columns" },
  ];

  var textarea = null;
  var builderRoot = null;
  var builderError = null;
  var state = null;
  var builderReady = false;

  // --- element defaults + parsing existing JSON into builder state ---

  function createElement(type) {
    switch (type) {
      case "text":
        return { type: "text", content: "", align: "", bold: false, italic: false, underline: false, strikethrough: false, size: 0 };
      case "heading":
        return { type: "heading", content: "" };
      case "divider":
        return { type: "divider", style: "", size: 0 };
      case "spacer":
        return { type: "spacer", height: 0 };
      case "feed":
        return { type: "feed", lines: 1 };
      case "cut":
        return { type: "cut", mode: "" };
      case "qrcode":
        return { type: "qrcode", content: "", size: 0, error_correction: "" };
      case "barcode":
        return { type: "barcode", content: "", symbology: "code128", height: 0, show_text: false };
      case "asset":
        return { type: "asset", name: "", width: 0, align: "" };
      case "image":
        return { type: "image", data: "" };
      case "table":
        return { type: "table", headersRaw: "", rowsRaw: "" };
      case "list":
        return { type: "list", kind: "", items: [] };
      case "columns":
        return { type: "columns", columns: [{ weight: 0, elements: [] }] };
      default:
        throw new Error("unsupported element type: " + type);
    }
  }

  // Parses one wire-format element (as decoded by receipt.Receipt's own
  // "type" discriminator) into builder state, recursing into columns.
  // Throws for any unrecognized type, at any depth — the caller (
  // parseInitialState) uses that to fall back to raw-JSON mode rather than
  // silently dropping content the builder can't represent.
  function normalizeElement(raw) {
    if (!raw || typeof raw.type !== "string") {
      throw new Error("invalid element");
    }
    var base = createElement(raw.type);
    switch (raw.type) {
      case "text":
        if ("content" in raw) base.content = raw.content;
        if ("align" in raw) base.align = raw.align;
        base.bold = !!raw.bold;
        base.italic = !!raw.italic;
        base.underline = !!raw.underline;
        base.strikethrough = !!raw.strikethrough;
        if ("size" in raw) base.size = raw.size;
        break;
      case "heading":
        if ("content" in raw) base.content = raw.content;
        break;
      case "divider":
        if ("style" in raw) base.style = raw.style;
        if ("size" in raw) base.size = raw.size;
        break;
      case "spacer":
        if ("height" in raw) base.height = raw.height;
        break;
      case "feed":
        if ("lines" in raw) base.lines = raw.lines;
        break;
      case "cut":
        if ("mode" in raw) base.mode = raw.mode;
        break;
      case "qrcode":
        if ("content" in raw) base.content = raw.content;
        if ("size" in raw) base.size = raw.size;
        if ("error_correction" in raw) base.error_correction = raw.error_correction;
        break;
      case "barcode":
        if ("content" in raw) base.content = raw.content;
        if ("symbology" in raw) base.symbology = raw.symbology;
        if ("height" in raw) base.height = raw.height;
        base.show_text = !!raw.show_text;
        break;
      case "asset":
        if ("name" in raw) base.name = raw.name;
        if ("width" in raw) base.width = raw.width;
        if ("align" in raw) base.align = raw.align;
        break;
      case "image":
        if (typeof raw.data === "string") base.data = raw.data;
        break;
      case "table":
        if (Array.isArray(raw.headers)) base.headersRaw = formatCSVLine(raw.headers);
        if (Array.isArray(raw.rows)) base.rowsRaw = raw.rows.map(formatCSVLine).join("\n");
        break;
      case "list":
        if ("kind" in raw) base.kind = raw.kind;
        if (Array.isArray(raw.items)) {
          base.items = raw.items.map(function (item) {
            return { content: item.content || "", checked: !!item.checked, indent: item.indent || 0 };
          });
        }
        break;
      case "columns":
        if (Array.isArray(raw.columns)) {
          base.columns = raw.columns.map(function (col) {
            return {
              weight: col.weight || 0,
              elements: Array.isArray(col.elements) ? col.elements.map(normalizeElement) : [],
            };
          });
        }
        break;
    }
    return base;
  }

  function parseInitialState(value) {
    var trimmed = value.trim();
    if (trimmed === "") {
      return { copies: 1, elements: [] };
    }
    try {
      var parsed = JSON.parse(trimmed);
      if (!parsed || typeof parsed !== "object" || !Array.isArray(parsed.elements)) {
        return null;
      }
      return {
        copies: typeof parsed.copies === "number" ? parsed.copies : 1,
        elements: parsed.elements.map(normalizeElement),
      };
    } catch (e) {
      return null;
    }
  }

  // --- builder state -> wire-format JSON ---

  // RFC 4180, so parse and format are exact inverses: a bare split/join
  // can express neither a cell containing a comma nor an empty one, and
  // corrupts such a receipt just by loading it into the builder.
  function parseCSV(text) {
    var src = String(text || "");
    var rows = [];
    var row = [];
    var field = "";
    var quoted = false;
    var quotedField = false;
    var rowIsBlank = true;

    function pushField() {
      var value = quotedField ? field : field.trim();
      if (quotedField || value !== "") {
        rowIsBlank = false;
      }
      row.push(value);
      field = "";
      quotedField = false;
    }

    function pushRow() {
      var sawSeparator = row.length > 0;
      pushField();
      if (!rowIsBlank || sawSeparator) {
        rows.push(row);
      }
      row = [];
      rowIsBlank = true;
    }

    for (var i = 0; i < src.length; i++) {
      var ch = src.charAt(i);
      if (quoted) {
        if (ch !== '"') {
          field += ch;
        } else if (src.charAt(i + 1) === '"') {
          field += '"';
          i++;
        } else {
          quoted = false;
        }
        continue;
      }
      if (ch === '"' && field.trim() === "") {
        quoted = true;
        quotedField = true;
        field = "";
      } else if (ch === ",") {
        pushField();
      } else if (ch === "\n") {
        pushRow();
      } else if (ch !== "\r") {
        field += ch;
      }
    }
    pushRow();
    return rows;
  }

  // Empty fields are written as "" so a row of them stays a row rather
  // than a blank line parseCSV drops.
  function formatCSVLine(fields) {
    return fields
      .map(function (f) {
        var s = String(f == null ? "" : f);
        if (s === "" || s !== s.trim() || /[",\n\r]/.test(s)) {
          return '"' + s.replace(/"/g, '""') + '"';
        }
        return s;
      })
      .join(",");
  }

  function splitCSVLine(line) {
    var rows = parseCSV(line);
    return rows.length > 0 ? rows[0] : [];
  }

  function splitTableRows(text) {
    return parseCSV(text);
  }

  function serializeElements(elements) {
    return elements.map(serializeElement);
  }

  function serializeElement(el) {
    switch (el.type) {
      case "text":
        return { type: "text", content: el.content, align: el.align, bold: el.bold, italic: el.italic, underline: el.underline, strikethrough: el.strikethrough, size: el.size };
      case "heading":
        return { type: "heading", content: el.content };
      case "divider":
        return { type: "divider", style: el.style, size: el.size };
      case "spacer":
        return { type: "spacer", height: el.height };
      case "feed":
        return { type: "feed", lines: el.lines };
      case "cut":
        return { type: "cut", mode: el.mode };
      case "qrcode":
        return { type: "qrcode", content: el.content, size: el.size, error_correction: el.error_correction };
      case "barcode":
        return { type: "barcode", content: el.content, symbology: el.symbology, height: el.height, show_text: el.show_text };
      case "asset":
        return { type: "asset", name: el.name, width: el.width, align: el.align };
      case "image":
        return { type: "image", data: el.data };
      case "table":
        return { type: "table", headers: splitCSVLine(el.headersRaw), rows: splitTableRows(el.rowsRaw) };
      case "list":
        return {
          type: "list",
          kind: el.kind,
          items: el.items.map(function (item) {
            return { content: item.content, checked: item.checked, indent: item.indent };
          }),
        };
      case "columns":
        return {
          type: "columns",
          columns: el.columns.map(function (col) {
            return { weight: col.weight, elements: serializeElements(col.elements) };
          }),
        };
      default:
        return null;
    }
  }

  function markStale() {
    var notice = document.getElementById("stale-notice");
    var img = document.querySelector(".preview-area img");
    if (notice && img) {
      notice.hidden = false;
    }
  }

  function syncTextarea() {
    var payload = { version: 1, copies: state.copies, elements: serializeElements(state.elements) };
    textarea.value = JSON.stringify(payload, null, 2);
    if (builderReady) {
      markStale();
    }
  }

  // A structural change (add/remove/move an element or column) rebuilds
  // the whole builder DOM from state; a leaf field's own input handler
  // updates state and calls syncTextarea() directly instead, so typing in
  // a text field doesn't lose focus on every keystroke.
  function handleStructuralChange() {
    if (builderError && state.elements.length > 0) {
      builderError.hidden = true;
    }
    syncTextarea();
    render();
  }

  // --- generic field controls ---

  function wrapField(labelText, input) {
    var label = document.createElement("label");
    label.className = "field";
    label.appendChild(document.createTextNode(labelText));
    label.appendChild(input);
    return label;
  }

  function fieldRow(fields) {
    var row = document.createElement("div");
    row.className = "field-row";
    fields.forEach(function (f) {
      row.appendChild(f);
    });
    return row;
  }

  function textField(labelText, el, key) {
    var input = document.createElement("input");
    input.type = "text";
    input.value = el[key] || "";
    input.addEventListener("input", function () {
      el[key] = input.value;
      syncTextarea();
    });
    return wrapField(labelText, input);
  }

  function numberField(labelText, el, key, opts) {
    opts = opts || {};
    var input = document.createElement("input");
    input.type = "number";
    if (opts.min !== undefined) input.min = String(opts.min);
    if (opts.max !== undefined) input.max = String(opts.max);
    input.value = el[key] || 0;
    input.addEventListener("input", function () {
      el[key] = input.value === "" ? 0 : Number(input.value);
      syncTextarea();
    });
    return wrapField(labelText, input);
  }

  function selectField(labelText, el, key, options) {
    var select = document.createElement("select");
    var current = el[key] || "";
    var known = false;
    options.forEach(function (pair) {
      var opt = document.createElement("option");
      opt.value = pair[0];
      opt.textContent = pair[1];
      if (pair[0] === current) {
        known = true;
      }
      select.appendChild(opt);
    });
    // receipt/*.go accepts values this builder doesn't offer (List's
    // "bullet", say); without an option for it the control renders blank
    // and misreports the element's state.
    if (!known) {
      var extra = document.createElement("option");
      extra.value = current;
      extra.textContent = current + " (current)";
      select.insertBefore(extra, select.firstChild);
    }
    select.value = current;
    select.addEventListener("change", function () {
      el[key] = select.value;
      syncTextarea();
    });
    return wrapField(labelText, select);
  }

  function checkboxField(labelText, el, key) {
    var wrapper = document.createElement("label");
    wrapper.className = "field-checkbox";
    var input = document.createElement("input");
    input.type = "checkbox";
    input.checked = !!el[key];
    input.addEventListener("change", function () {
      el[key] = input.checked;
      syncTextarea();
    });
    wrapper.appendChild(input);
    wrapper.appendChild(document.createTextNode(labelText));
    return wrapper;
  }

  function removeButton(onClick) {
    var btn = document.createElement("button");
    btn.type = "button";
    btn.className = "button-danger";
    btn.textContent = "Remove";
    btn.addEventListener("click", onClick);
    return btn;
  }

  // --- per-type field sets ---

  var ALIGN_OPTIONS = [
    ["", "Default (left)"],
    ["left", "Left"],
    ["center", "Center"],
    ["right", "Right"],
  ];

  function renderTextFields(el) {
    var wrapper = document.createElement("div");

    var contentLabel = document.createElement("label");
    contentLabel.className = "field";
    contentLabel.appendChild(document.createTextNode("Content"));
    var content = document.createElement("textarea");
    content.rows = 2;
    content.value = el.content || "";
    content.addEventListener("input", function () {
      el.content = content.value;
      syncTextarea();
    });
    contentLabel.appendChild(content);
    wrapper.appendChild(fieldRow([contentLabel]));

    wrapper.appendChild(fieldRow([selectField("Align", el, "align", ALIGN_OPTIONS), numberField("Size", el, "size", { min: 0, max: 100 })]));
    wrapper.appendChild(
      fieldRow([checkboxField("Bold", el, "bold"), checkboxField("Italic", el, "italic"), checkboxField("Underline", el, "underline"), checkboxField("Strikethrough", el, "strikethrough")])
    );

    return wrapper;
  }

  function renderImageField(el) {
    var wrapper = document.createElement("div");

    var label = document.createElement("label");
    label.className = "field";
    label.appendChild(document.createTextNode("Image file"));
    var input = document.createElement("input");
    input.type = "file";
    input.accept = "image/png,image/jpeg,image/gif,image/bmp,image/webp";
    label.appendChild(input);
    wrapper.appendChild(fieldRow([label]));

    var status = document.createElement("p");
    status.className = "dropzone-hint";
    status.textContent = el.data ? "A file is set for this element." : "No file selected yet.";
    wrapper.appendChild(status);

    input.addEventListener("change", function () {
      var file = input.files && input.files[0];
      if (!file) {
        return;
      }
      var reader = new FileReader();
      reader.onload = function () {
        // Image.Data ([]byte) decodes from plain base64 — strip the
        // "data:...;base64," prefix readAsDataURL adds.
        var result = String(reader.result || "");
        var comma = result.indexOf(",");
        el.data = comma === -1 ? "" : result.slice(comma + 1);
        status.textContent = "Selected: " + file.name;
        syncTextarea();
      };
      reader.readAsDataURL(file);
    });

    return wrapper;
  }

  function renderTableFields(el) {
    var wrapper = document.createElement("div");

    var headersLabel = document.createElement("label");
    headersLabel.className = "field";
    headersLabel.appendChild(document.createTextNode('Headers (comma-separated; quote a cell containing a comma: "a, b")'));
    var headers = document.createElement("input");
    headers.type = "text";
    headers.value = el.headersRaw || "";
    headers.addEventListener("input", function () {
      el.headersRaw = headers.value;
      syncTextarea();
    });
    headersLabel.appendChild(headers);
    wrapper.appendChild(fieldRow([headersLabel]));

    var rowsLabel = document.createElement("label");
    rowsLabel.className = "field";
    rowsLabel.appendChild(document.createTextNode('Rows (one per line, comma-separated cells; "" is an empty cell)'));
    var rows = document.createElement("textarea");
    rows.rows = 4;
    rows.value = el.rowsRaw || "";
    rows.addEventListener("input", function () {
      el.rowsRaw = rows.value;
      syncTextarea();
    });
    rowsLabel.appendChild(rows);
    wrapper.appendChild(fieldRow([rowsLabel]));

    return wrapper;
  }

  function renderListItem(list, item, index) {
    var row = document.createElement("div");
    row.className = "field-row";

    var content = document.createElement("input");
    content.type = "text";
    content.value = item.content || "";
    content.addEventListener("input", function () {
      item.content = content.value;
      syncTextarea();
    });
    row.appendChild(wrapField("Content", content));

    var indent = document.createElement("input");
    indent.type = "number";
    indent.min = "0";
    indent.max = "8";
    indent.value = item.indent || 0;
    indent.addEventListener("input", function () {
      item.indent = indent.value === "" ? 0 : Number(indent.value);
      syncTextarea();
    });
    row.appendChild(wrapField("Indent", indent));

    row.appendChild(checkboxField("Checked (checkbox lists only)", item, "checked"));

    row.appendChild(
      removeButton(function () {
        list.items.splice(index, 1);
        handleStructuralChange();
      })
    );

    return row;
  }

  function renderListFields(el) {
    var wrapper = document.createElement("div");
    wrapper.appendChild(
      fieldRow([
        selectField("Kind", el, "kind", [
          ["", "Bullet (default)"],
          ["number", "Number"],
          ["checkbox", "Checkbox"],
        ]),
      ])
    );

    var items = document.createElement("div");
    items.className = "element-list";
    el.items.forEach(function (item, index) {
      items.appendChild(renderListItem(el, item, index));
    });
    wrapper.appendChild(items);

    var addBtn = document.createElement("button");
    addBtn.type = "button";
    addBtn.className = "button-secondary";
    addBtn.textContent = "Add item";
    addBtn.addEventListener("click", function () {
      el.items.push({ content: "", checked: false, indent: 0 });
      handleStructuralChange();
    });
    wrapper.appendChild(addBtn);

    return wrapper;
  }

  function renderColumnSlot(columns, col, index) {
    var slot = document.createElement("div");
    slot.className = "column-slot";

    var header = document.createElement("div");
    header.className = "column-slot-header";
    header.appendChild(numberField("Weight", col, "weight", { min: 0 }));

    var removeCol = removeButton(function () {
      columns.columns.splice(index, 1);
      handleStructuralChange();
    });
    removeCol.textContent = "Remove column";
    header.appendChild(removeCol);

    slot.appendChild(header);
    slot.appendChild(renderElementList(col.elements));
    return slot;
  }

  function renderColumnsFields(el) {
    var wrapper = document.createElement("div");

    el.columns.forEach(function (col, index) {
      wrapper.appendChild(renderColumnSlot(el, col, index));
    });

    var addBtn = document.createElement("button");
    addBtn.type = "button";
    addBtn.className = "button-secondary";
    addBtn.textContent = "Add column";
    addBtn.addEventListener("click", function () {
      el.columns.push({ weight: 0, elements: [] });
      handleStructuralChange();
    });
    wrapper.appendChild(addBtn);

    return wrapper;
  }

  function renderFields(el) {
    switch (el.type) {
      case "text":
        return renderTextFields(el);
      case "heading":
        return fieldRow([textField("Content", el, "content")]);
      case "divider":
        return fieldRow([
          selectField("Style", el, "style", [
            ["", "Default"],
            ["solid", "Solid"],
            ["dashed", "Dashed"],
          ]),
          numberField("Size", el, "size", { min: 0, max: 100 }),
        ]);
      case "spacer":
        return fieldRow([numberField("Height (dots)", el, "height", { min: 0, max: 10000 })]);
      case "feed":
        return fieldRow([numberField("Lines", el, "lines", { min: 1, max: 255 })]);
      case "cut":
        return fieldRow([
          selectField("Mode", el, "mode", [
            ["", "Printer default"],
            ["full", "Full"],
            ["partial", "Partial"],
          ]),
        ]);
      case "qrcode":
        return fieldRow([
          textField("Content", el, "content"),
          numberField("Size (dots)", el, "size", { min: 0 }),
          selectField("Error correction", el, "error_correction", [
            ["", "Default (medium)"],
            ["low", "Low"],
            ["medium", "Medium"],
            ["quartile", "Quartile"],
            ["high", "High"],
          ]),
        ]);
      case "barcode":
        return fieldRow([
          textField("Content", el, "content"),
          selectField("Symbology", el, "symbology", [
            ["code128", "Code 128"],
            ["ean13", "EAN-13"],
            ["ean8", "EAN-8"],
            ["upca", "UPC-A"],
            ["code39", "Code 39"],
            ["itf", "ITF"],
          ]),
          numberField("Height (dots)", el, "height", { min: 0, max: 10000 }),
          checkboxField("Show text", el, "show_text"),
        ]);
      case "asset":
        return fieldRow([textField("Name", el, "name"), numberField("Width (dots)", el, "width", { min: 0 }), selectField("Align", el, "align", ALIGN_OPTIONS)]);
      case "image":
        return renderImageField(el);
      case "table":
        return renderTableFields(el);
      case "list":
        return renderListFields(el);
      case "columns":
        return renderColumnsFields(el);
      default:
        return document.createElement("div");
    }
  }

  // --- element list (recursive: a column's elements re-enter here) ---

  function typeLabel(type) {
    for (var i = 0; i < ELEMENT_TYPES.length; i++) {
      if (ELEMENT_TYPES[i].type === type) {
        return ELEMENT_TYPES[i].label;
      }
    }
    return type;
  }

  function swap(arr, i, j) {
    var tmp = arr[i];
    arr[i] = arr[j];
    arr[j] = tmp;
  }

  function moveButton(label, enabled, onClick) {
    var btn = document.createElement("button");
    btn.type = "button";
    btn.className = "button-secondary";
    btn.textContent = label;
    btn.disabled = !enabled;
    btn.addEventListener("click", onClick);
    return btn;
  }

  function renderElementCard(el, index, siblings) {
    var card = document.createElement("div");
    card.className = "element-card" + (el.type === "columns" ? " columns" : "");

    var header = document.createElement("div");
    header.className = "element-card-header";

    var title = document.createElement("span");
    title.className = "element-card-title";
    title.textContent = typeLabel(el.type);
    header.appendChild(title);

    var actions = document.createElement("div");
    actions.className = "element-card-actions";
    actions.appendChild(
      moveButton("↑", index > 0, function () {
        swap(siblings, index, index - 1);
        handleStructuralChange();
      })
    );
    actions.appendChild(
      moveButton("↓", index < siblings.length - 1, function () {
        swap(siblings, index, index + 1);
        handleStructuralChange();
      })
    );
    actions.appendChild(
      removeButton(function () {
        siblings.splice(index, 1);
        handleStructuralChange();
      })
    );
    header.appendChild(actions);

    card.appendChild(header);
    card.appendChild(renderFields(el));
    return card;
  }

  function renderAddElementRow(elements) {
    var row = document.createElement("div");
    row.className = "add-element-row";

    var select = document.createElement("select");
    ELEMENT_TYPES.forEach(function (t) {
      var opt = document.createElement("option");
      opt.value = t.type;
      opt.textContent = t.label;
      select.appendChild(opt);
    });

    var btn = document.createElement("button");
    btn.type = "button";
    btn.className = "button-secondary";
    btn.textContent = "Add element";
    btn.addEventListener("click", function () {
      elements.push(createElement(select.value));
      handleStructuralChange();
    });

    row.appendChild(select);
    row.appendChild(btn);
    return row;
  }

  function renderElementList(elements) {
    var wrapper = document.createElement("div");
    wrapper.appendChild(renderAddElementRow(elements));

    var list = document.createElement("div");
    list.className = "element-list";
    elements.forEach(function (el, index) {
      list.appendChild(renderElementCard(el, index, elements));
    });
    wrapper.appendChild(list);

    return wrapper;
  }

  // --- top-level builder chrome ---

  function createRawToggle() {
    var wrapper = document.createElement("div");
    wrapper.className = "builder-toggle";
    var btn = document.createElement("button");
    btn.type = "button";
    btn.className = "button-secondary";
    btn.textContent = "Use raw JSON instead";
    btn.addEventListener("click", function () {
      builderRoot.remove();
      textarea.parentElement.classList.remove("js-hidden");
    });
    wrapper.appendChild(btn);
    return wrapper;
  }

  function createCopiesField() {
    var input = document.createElement("input");
    input.type = "number";
    input.min = "0";
    input.max = "100";
    input.value = state.copies;
    input.addEventListener("input", function () {
      state.copies = input.value === "" ? 0 : Number(input.value);
      syncTextarea();
    });
    var label = document.createElement("label");
    label.className = "field copies-field";
    label.appendChild(document.createTextNode("Copies"));
    label.appendChild(input);
    return label;
  }

  // A hidden textarea is out of constraint validation's reach, and empty
  // state still serializes to JSON Receipt.Validate accepts — without
  // this an untouched form prints blank paper.
  function validateOnSubmit(event) {
    builderError.hidden = state.elements.length > 0;
    if (builderError.hidden) {
      return;
    }
    event.preventDefault();
    builderError.scrollIntoView({ block: "nearest" });
  }

  function render() {
    builderRoot.innerHTML = "";
    builderRoot.appendChild(builderError);
    builderRoot.appendChild(createRawToggle());
    builderRoot.appendChild(createCopiesField());
    builderRoot.appendChild(renderElementList(state.elements));
  }

  function setupBuilderUI(initialState) {
    state = initialState;
    textarea.parentElement.classList.add("js-hidden");

    builderError = document.createElement("p");
    builderError.className = "validation-message";
    builderError.textContent = "Add at least one element before submitting.";
    builderError.hidden = true;

    builderRoot = document.createElement("div");
    builderRoot.className = "element-builder";
    textarea.parentElement.insertAdjacentElement("beforebegin", builderRoot);

    if (textarea.form) {
      textarea.form.addEventListener("submit", validateOnSubmit);
    }

    handleStructuralChange();
    builderReady = true;
  }

  function init() {
    textarea = document.querySelector('textarea[name="receipt"]');
    if (!textarea) {
      return;
    }
    // Harmless in builder mode: the textarea is hidden and only ever
    // written to programmatically there, which doesn't fire "input".
    textarea.addEventListener("input", markStale);

    var initialState = parseInitialState(textarea.value);
    if (initialState === null) {
      return;
    }
    setupBuilderUI(initialState);
  }

  document.addEventListener("DOMContentLoaded", init);
})();
