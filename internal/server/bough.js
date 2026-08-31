// Draws what layout.js worked out, and handles reading and filtering.
//
// The graph carries no positions, colours or sizes. Everything visual is
// decided here, so the core stays a description of the work rather than a
// description of a picture of it.
(function () {
  "use strict";

  var graph = window.BOUGH;
  var L = window.BoughLayout;
  if (!graph || !L) return;

  var SVGNS = "http://www.w3.org/2000/svg";

  // Below this a sitting is a loose end rather than a piece of work.
  var SUBSTANTIAL = 5;

  var view = { x: 0, y: 0, scale: 1 };
  var model = null;
  var selected = null;

  var filters = {
    file: "",
    from: null,
    to: null,
    substantialOnly: false,
    hardOnly: false
  };

  var stage = document.getElementById("stage");
  var host = document.getElementById("tree");
  var grid = document.getElementById("grid");

  // ---- building the drawing ----------------------------------------------

  function el(tag, attrs) {
    var node = document.createElementNS(SVGNS, tag);
    for (var k in attrs) {
      if (attrs[k] != null) node.setAttribute(k, attrs[k]);
    }
    return node;
  }

  function draw() {
    model = L.build(graph, "reading");
    host.textContent = "";

    var svg = el("svg", {
      viewBox: "0 0 " + model.width + " " + model.height,
      preserveAspectRatio: "xMidYMin meet"
    });

    var root = el("g", { id: "canvas" });
    svg.appendChild(root);

    // Vines sit behind everything, since they are a note about the work
    // rather than part of its structure.
    var vines = el("g", { class: "vines" });
    model.vines.forEach(function (vine) {
      var p = el("path", {
        d: vine.path,
        class: "vine",
        "stroke-width": Math.min(2.4, 0.8 + vine.weight / 30)
      });
      p.appendChild(title(vine.files.length + " files picked up again"));
      vines.appendChild(p);
    });
    root.appendChild(vines);

    root.appendChild(el("path", { d: model.trunk, class: "trunk" }));

    model.limbs.forEach(function (limb) {
      root.appendChild(drawLimb(limb));
    });

    host.appendChild(svg);
    apply();
    fit();
  }

  function drawLimb(limb) {
    var g = el("g", { class: "limb", "data-id": limb.id });
    var goal = limb.goal;

    // The limb itself, a short stub off the trunk that its branches hang from.
    var reach = 46;
    g.appendChild(el("path", {
      d: "M" + model.trunkX + "," + limb.y + " l" + reach + ",0",
      class: "limb-stub",
      "stroke-width": limb.width
    }));
    g.appendChild(el("path", {
      d: "M" + model.trunkX + "," + limb.y + " l" + (-reach) + ",0",
      class: "limb-stub",
      "stroke-width": limb.width
    }));

    limb.branches.forEach(function (branch) {
      g.appendChild(drawBranch(branch));
    });

    // The date rides on the trunk, which is the only label showing at rest.
    var date = el("text", {
      x: model.trunkX,
      y: limb.y - limb.width / 2 - 12,
      class: "limb-date",
      "text-anchor": "middle"
    });
    date.textContent = goal.period || "";
    g.appendChild(date);

    return g;
  }

  function drawBranch(branch) {
    var g = el("g", {
      class: "branch" + (branch.hard ? " hard" : ""),
      "data-id": branch.id,
      tabindex: "0",
      role: "button"
    });

    g.appendChild(el("path", {
      d: branch.path,
      class: "branch-line",
      "stroke-width": branch.width
    }));

    var leaves = el("g", { class: "leaves" });
    branch.leaves.forEach(function (leaf) {
      leaves.appendChild(el("circle", { cx: leaf.x, cy: leaf.y, r: leaf.r }));
    });
    g.appendChild(leaves);

    // A generous invisible path so a two pixel branch is still easy to hit.
    g.appendChild(el("path", { d: branch.path, class: "hit" }));

    var label = el("text", {
      x: branch.tip.x + branch.side * 16,
      y: branch.tip.y + 4,
      class: "branch-label",
      "text-anchor": branch.side > 0 ? "start" : "end"
    });
    label.textContent = clip(branch.task.label || "", 38);
    g.appendChild(label);

    g.addEventListener("click", function (e) { e.stopPropagation(); open(branch); });
    g.addEventListener("keydown", function (e) {
      if (e.key === "Enter" || e.key === " ") { e.preventDefault(); open(branch); }
    });
    return g;
  }

  function title(text) {
    var t = el("title", {});
    t.textContent = text;
    return t;
  }

  // ---- moving around ------------------------------------------------------

  function apply() {
    var canvas = host.querySelector("#canvas");
    if (!canvas) return;
    canvas.setAttribute(
      "transform",
      "translate(" + view.x + "," + view.y + ") scale(" + view.scale + ")"
    );
    // The grid travels with the drawing, so panning reads as crossing a
    // surface rather than sliding a picture over a fixed backdrop.
    grid.style.backgroundPosition = view.x + "px " + view.y + "px";
    grid.style.backgroundSize =
      [120, 120, 24, 24].map(function (n) {
        var s = n * view.scale;
        return s + "px " + s + "px";
      }).join(", ");
  }

  // fit shows the whole tree, which is the first thing anyone should see.
  function fit() {
    if (!model || !model.height) return;
    var box = stage.getBoundingClientRect();
    var margin = 90;
    view.scale = Math.min(
      1.1,
      (box.height - margin * 2) / model.height,
      (box.width - margin * 2) / model.width
    );
    view.x = (box.width - model.width * view.scale) / 2;
    view.y = margin;
    apply();
  }

  function bindPanZoom() {
    var dragging = false;
    var from = { x: 0, y: 0 };

    stage.addEventListener("pointerdown", function (e) {
      if (e.target.closest(".branch")) return;
      dragging = true;
      from = { x: e.clientX - view.x, y: e.clientY - view.y };
      stage.classList.add("dragging");
      stage.setPointerCapture(e.pointerId);
    });

    stage.addEventListener("pointermove", function (e) {
      if (!dragging) return;
      view.x = e.clientX - from.x;
      view.y = e.clientY - from.y;
      apply();
    });

    ["pointerup", "pointercancel"].forEach(function (name) {
      stage.addEventListener(name, function () {
        dragging = false;
        stage.classList.remove("dragging");
      });
    });

    stage.addEventListener("wheel", function (e) {
      e.preventDefault();
      var box = stage.getBoundingClientRect();
      var mx = e.clientX - box.left;
      var my = e.clientY - box.top;
      var factor = Math.exp(-e.deltaY * 0.0015);
      var next = Math.min(3, Math.max(0.15, view.scale * factor));

      // Keep whatever is under the pointer under the pointer.
      view.x = mx - (mx - view.x) * (next / view.scale);
      view.y = my - (my - view.y) * (next / view.scale);
      view.scale = next;
      apply();
    }, { passive: false });

    stage.addEventListener("click", function (e) {
      if (!e.target.closest(".branch")) close();
    });

    window.addEventListener("resize", function () {
      if (!selected) fit();
    });
  }

  // ---- reading ------------------------------------------------------------

  function open(branch) {
    selected = branch;
    document.body.classList.add("reading");

    host.querySelectorAll(".branch.open").forEach(function (n) {
      n.classList.remove("open");
    });
    host.querySelector('.branch[data-id="' + cssEscape(branch.id) + '"]').classList.add("open");

    var task = branch.task;
    var panel = document.getElementById("reader");
    panel.textContent = "";

    var head = node("header", "reader-head");
    head.appendChild(node("h2", null, task.label || "(unnamed)"));
    head.appendChild(node("p", "reader-figures", figures(task.stats)));
    panel.appendChild(head);

    if (task.stats.churn > 1 && task.stats.churnFile) {
      panel.appendChild(node("p", "reader-churn",
        "kept coming back to " + baseName(task.stats.churnFile) +
        " (" + task.stats.churn + " times)"));
    }

    var list = node("ol", "turns");
    task.turns.forEach(function (turn) {
      var li = node("li", "turn");
      li.appendChild(node("time", "when", when(turn.at)));
      // Prompts are the reader's own words and the reason to look, so they
      // are never trimmed.
      li.appendChild(node("p", "said", turn.text));
      (turn.delegated || []).forEach(function (job) {
        li.appendChild(node("p", "handoff", job.description));
      });
      list.appendChild(li);
    });
    panel.appendChild(list);
    panel.scrollTop = 0;
  }

  function close() {
    selected = null;
    document.body.classList.remove("reading");
    host.querySelectorAll(".branch.open").forEach(function (n) {
      n.classList.remove("open");
    });
  }

  // ---- filtering ----------------------------------------------------------

  // Filters fade rather than remove. Dropping a branch would change the shape,
  // and the shape is the thing being looked at.
  function refilter() {
    var needle = filters.file.toLowerCase();
    var anyShown = false;

    model.limbs.forEach(function (limb) {
      var node = host.querySelector('.limb[data-id="' + cssEscape(limb.id) + '"]');
      if (!node) return;

      var stats = limb.goal.stats;
      var limbOut =
        (filters.substantialOnly && (stats.turns || 0) < SUBSTANTIAL) ||
        (filters.from && day(stats.end) < filters.from) ||
        (filters.to && day(stats.start) > filters.to);

      var shown = 0;
      limb.branches.forEach(function (branch) {
        var b = host.querySelector('.branch[data-id="' + cssEscape(branch.id) + '"]');
        if (!b) return;
        var out = limbOut ||
          (needle && !touches(branch.task, needle)) ||
          (filters.hardOnly && !branch.hard);
        b.classList.toggle("faded", Boolean(out));
        if (!out) shown++;
      });

      node.classList.toggle("faded", limbOut || shown === 0);
      if (shown) anyShown = true;
    });

    document.getElementById("clear").hidden = !filtering();
    document.body.classList.toggle("nothing", !anyShown && filtering());
  }

  function touches(task, needle) {
    var files = (task.stats && task.stats.topFiles) || [];
    for (var i = 0; i < files.length; i++) {
      if (files[i].path.toLowerCase().indexOf(needle) !== -1) return true;
    }
    return false;
  }

  function filtering() {
    return Boolean(filters.file || filters.from || filters.to ||
      filters.substantialOnly || filters.hardOnly);
  }

  function bindRail() {
    var file = document.getElementById("f-file");
    var from = document.getElementById("f-from");
    var to = document.getElementById("f-to");
    var size = document.getElementById("f-size");
    var hard = document.getElementById("f-hard");

    if (graph.goals.length) {
      var first = day(graph.goals[0].stats.start);
      var last = day(graph.goals[graph.goals.length - 1].stats.end);
      [from, to].forEach(function (input) { input.min = first; input.max = last; });
    }

    file.addEventListener("input", function () { filters.file = file.value.trim(); refilter(); });
    from.addEventListener("change", function () { filters.from = from.value || null; refilter(); });
    to.addEventListener("change", function () { filters.to = to.value || null; refilter(); });
    size.addEventListener("change", function () { filters.substantialOnly = size.checked; refilter(); });
    hard.addEventListener("change", function () { filters.hardOnly = hard.checked; refilter(); });

    document.getElementById("clear").addEventListener("click", function () {
      file.value = ""; from.value = ""; to.value = "";
      size.checked = false; hard.checked = false;
      filters = { file: "", from: null, to: null, substantialOnly: false, hardOnly: false };
      refilter();
    });

    // Opening one control closes the others, so the rail never becomes a wall.
    document.querySelectorAll(".rail-item").forEach(function (item) {
      item.querySelector(".rail-btn").addEventListener("click", function () {
        var wasOpen = item.classList.contains("open");
        document.querySelectorAll(".rail-item.open").forEach(function (o) {
          o.classList.remove("open");
        });
        if (!wasOpen) {
          item.classList.add("open");
          var input = item.querySelector("input");
          if (input && input.type !== "checkbox") input.focus();
        }
      });
    });
  }

  // ---- odds and ends ------------------------------------------------------

  function node(tag, className, text) {
    var n = document.createElement(tag);
    if (className) n.className = className;
    if (text != null) n.textContent = text;
    return n;
  }

  function count(n, one) {
    return n + " " + (n === 1 ? one : one + "s");
  }

  // Fields are left out of the graph when they are zero, so nothing here may
  // assume a key is present.
  function figures(stats) {
    var bits = [count(stats.turns || 0, "prompt")];
    if (stats.edits) bits.push(count(stats.edits, "change"));
    if (stats.errors) bits.push(count(stats.errors, "failure"));
    return bits.join("  ·  ");
  }

  function duration(minutes) {
    if (!minutes) return "a moment";
    if (minutes < 60) return minutes + " min";
    var hours = Math.floor(minutes / 60);
    var rest = minutes % 60;
    return hours < 3 && rest ? hours + "h " + rest + "m" : hours + "h";
  }

  function when(iso) {
    return new Date(iso).toLocaleString([], {
      month: "short", day: "numeric", hour: "2-digit", minute: "2-digit"
    });
  }

  function day(iso) { return iso ? iso.slice(0, 10) : ""; }

  function baseName(path) {
    var parts = String(path).split(/[\\/]/);
    return parts[parts.length - 1];
  }

  function clip(s, n) {
    s = s.replace(/\s+/g, " ").trim();
    return s.length > n ? s.slice(0, n - 1) + "…" : s;
  }

  // Ids are generated as g1.t2, and the dot needs escaping in a selector.
  function cssEscape(s) {
    return String(s).replace(/([.:#[\]])/g, "\\$1");
  }

  function chrome() {
    document.getElementById("name").textContent = graph.project.name;
    document.getElementById("where").textContent = graph.project.path;

    var t = graph.totals;
    var foot = document.getElementById("foot");
    [
      [t.turns || 0, "prompts"],
      [graph.goals.length, "sittings"],
      [t.edits || 0, "changes"],
      [t.files || 0, "files"],
      [duration(t.activeMinutes), "at the keyboard"]
    ].forEach(function (pair) {
      var span = node("span");
      span.appendChild(node("b", null, String(pair[0])));
      span.appendChild(document.createTextNode(" " + pair[1]));
      foot.appendChild(span);
    });
  }

  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape") close();
  });

  chrome();
  if (!graph.goals.length) {
    document.body.classList.add("nothing");
    return;
  }
  draw();
  bindPanZoom();
  bindRail();
})();
