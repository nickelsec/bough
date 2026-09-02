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

  // Below this a day is a loose end rather than a piece of work.
  var SUBSTANTIAL = 5;

  var view = { x: 0, y: 0, scale: 1 };
  var model = null;
  var selected = null;

  // Fitting a wide project to a window scales it to about a third, at which a
  // 12px circle draws at four pixels and the nodes read as dots rather than
  // as nodes. Positions still scale; the nodes themselves are held at a size
  // you can see and hit. Measured: at the tightest opening scale the nearest
  // two circles are 12px apart, so a 9px floor never makes them touch.
  var FLOOR = { prompt: 9, task: 13, day: 20 };
  var sized = [];

  // The scale that shows the whole diagram. The bar reads against this rather
  // than against the raw transform, so 100% means "all of it", which is what
  // resetting gives you and what the number ought to agree with.
  var whole = 1;
  var LIMIT = { min: 0.15, max: 3 };

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
  var pop = document.getElementById("pop");

  // ---- building the drawing ----------------------------------------------

  function el(tag, attrs) {
    var node = document.createElementNS(SVGNS, tag);
    for (var k in attrs) {
      if (attrs[k] != null) node.setAttribute(k, attrs[k]);
    }
    return node;
  }

  function line(x1, y1, x2, y2, className) {
    return el("line", { x1: x1, y1: y1, x2: x2, y2: y2, class: className });
  }

  function draw() {
    model = L.build(graph, "reading");
    host.textContent = "";
    sized = [];

    var svg = el("svg", {
      viewBox: "0 0 " + model.width + " " + model.height,
      preserveAspectRatio: "xMidYMid meet"
    });

    var root = el("g", { id: "canvas" });
    svg.appendChild(root);

    // Links sit behind everything, since they are a note about the work
    // rather than part of its structure.
    var links = el("g", { class: "links" });
    model.links.forEach(function (link) {
      var p = el("path", {
        d: link.path,
        class: "link",
        "data-from": link.from,
        "data-to": link.to
      });
      p.appendChild(title(link.files.length + " files picked up again"));
      links.appendChild(p);
    });
    root.appendChild(links);

    // The spine runs through every day, and time runs along it. It starts on
    // a dot and ends in an arrow, so it reads as a line that began somewhere
    // and is still going rather than one that was simply cut off at both ends.
    root.appendChild(line(
      model.spine.x1, model.spine.y, model.spine.x2, model.spine.y, "spine"
    ));
    var start = el("circle", {
      cx: model.spine.x1, cy: model.spine.y, class: "spine-start"
    });
    round(start, { x: model.spine.x1, y: model.spine.y }, 8, 8);
    root.appendChild(start);

    var arrow = el("path", { class: "spine-end" });
    tip(arrow, model.spine.x2, model.spine.y, 13);
    root.appendChild(arrow);

    // Every line first, so no connector is ever drawn over a node.
    var wires = el("g", { class: "wires" });
    model.days.forEach(function (day) {
      day.tasks.forEach(function (task) {
        var stem = line(day.x, day.y, task.x, task.y, "wire");
        stem.setAttribute("data-for", task.id);
        wires.appendChild(stem);
        task.prompts.forEach(function (p) {
          var twig = line(task.x, task.y, p.x, p.y, "twig");
          twig.setAttribute("data-for", p.id);
          wires.appendChild(twig);
        });
      });
    });
    root.appendChild(wires);

    model.days.forEach(function (day) {
      root.appendChild(drawDay(day));
    });

    host.appendChild(svg);
    apply();
    fit();
    grow(svg);
  }

  // grow runs the load sequence once. The dash length has to be the real
  // width of the spine or the line either finishes early or never arrives.
  function grow(svg) {
    var spine = svg.querySelector(".spine");
    if (!spine) return;
    svg.setAttribute("style", "--run:" + (model.spine.x2 - model.spine.x1));
    document.body.classList.add("growing");
    setTimeout(function () {
      document.body.classList.remove("growing");
    }, 1000);
  }

  function drawDay(day) {
    var g = el("g", { class: "day", "data-id": day.id });

    day.tasks.forEach(function (task) {
      g.appendChild(drawTask(task));
    });

    var box = el("rect", { rx: 2, class: "node day-node" });
    square(box, day, day.size, FLOOR.day);
    day.node = box;
    g.appendChild(box);

    // The date is the only label showing at rest, since it is the one thing
    // you need to read the diagram left to right.
    var date = el("text", {
      x: day.x,
      y: day.y + day.size / 2 + 20,
      class: "day-date",
      "text-anchor": "middle"
    });
    date.textContent = day.goal.period || "";
    g.appendChild(date);

    bind(g, day);
    return g;
  }

  function drawTask(task) {
    var g = el("g", {
      class: "task" + (task.hard ? " hard" : ""),
      "data-id": task.id,
      tabindex: "0",
      role: "button"
    });

    var box = el("rect", { rx: 2, class: "node task-node" });
    square(box, task, task.size, FLOOR.task);
    task.node = box;
    g.appendChild(box);

    task.prompts.forEach(function (p) {
      var c = el("circle", {
        cx: p.x, cy: p.y,
        class: "node prompt-node",
        "data-id": p.id,
        tabindex: "0",
        role: "button"
      });
      round(c, p, p.r * 2, FLOOR.prompt);
      p.node = c;
      bind(c, p);
      g.appendChild(c);
    });

    var label = el("text", {
      // Above the box on the upper side, below it on the lower, so a label
      // never sits on top of the prompts hanging off the same task.
      x: task.x,
      y: task.side < 0 ? task.y - task.size / 2 - 9 : task.y + task.size / 2 + 15,
      class: "task-label",
      "text-anchor": "middle"
    });
    label.textContent = clip(task.task.label || "", 34);
    g.appendChild(label);

    bind(g, task);
    return g;
  }

  // square and round remember what a node should be so resize can redraw it.
  function square(node, at, natural, floor) {
    sized.push({ node: node, at: at, natural: natural, floor: floor, box: true });
    resize(sized[sized.length - 1]);
  }

  function round(node, at, natural, floor) {
    sized.push({ node: node, at: at, natural: natural, floor: floor, box: false });
    resize(sized[sized.length - 1]);
  }

  function tip(node, x, y, natural) {
    sized.push({ node: node, at: { x: x, y: y }, natural: natural, floor: natural, arrow: true });
    resize(sized[sized.length - 1]);
  }

  // resize draws one node at its natural size, or at the floor if the current
  // zoom would take it below what can be seen.
  function resize(item) {
    var on = Math.max(item.natural, item.floor / view.scale);
    var half = on / 2;
    if (item.arrow) {
      var x = item.at.x;
      var y = item.at.y;
      item.node.setAttribute("d",
        "M" + (x - on) + "," + (y - on * 0.46) +
        " L" + x + "," + y +
        " L" + (x - on) + "," + (y + on * 0.46));
    } else if (item.box) {
      item.node.setAttribute("x", item.at.x - half);
      item.node.setAttribute("y", item.at.y - half);
      item.node.setAttribute("width", on);
      item.node.setAttribute("height", on);
    } else {
      item.node.setAttribute("r", half);
    }
  }

  function bind(node, item) {
    node.addEventListener("click", function (e) {
      e.stopPropagation();
      choose(item);
    });
    node.addEventListener("keydown", function (e) {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        choose(item);
      }
    });
    node.addEventListener("pointerenter", function () { enter(item); });
    node.addEventListener("pointerleave", function () { leave(item); });
    // Reaching a node by keyboard shows the same note as reaching it by
    // pointer, or the note is only there for people using a mouse.
    node.addEventListener("focus", function () { enter(item); });
    node.addEventListener("blur", function () { leave(item); });
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
    sized.forEach(resize);
    placePop();
    readout();
  }

  // fit shows the whole diagram, which is the first thing anyone should see.
  function fit() {
    if (!model || !model.height) return;
    var box = stage.getBoundingClientRect();
    var margin = 40;
    view.scale = Math.min(
      1.1,
      (box.height - margin * 2) / model.height,
      (box.width - margin * 2) / model.width
    );
    whole = view.scale;
    view.x = (box.width - model.width * view.scale) / 2;
    view.y = (box.height - model.height * view.scale) / 2;
    apply();
  }

  // zoomTo changes the scale about a point, which is the pointer for a wheel
  // and the middle of the view for a button.
  function zoomTo(next, ax, ay) {
    next = Math.min(LIMIT.max, Math.max(LIMIT.min, next));
    view.x = ax - (ax - view.x) * (next / view.scale);
    view.y = ay - (ay - view.y) * (next / view.scale);
    view.scale = next;
    apply();
  }

  function readout() {
    var now = document.getElementById("z-now");
    if (!now) return;
    now.textContent = Math.round((view.scale / whole) * 100) + "%";
    document.getElementById("z-in").disabled = view.scale >= LIMIT.max - 0.001;
    document.getElementById("z-out").disabled = view.scale <= LIMIT.min + 0.001;
  }

  function bindZoom() {
    var step = 1.3;
    function middle() {
      var box = stage.getBoundingClientRect();
      return [box.width / 2, box.height / 2];
    }
    document.getElementById("z-in").addEventListener("click", function () {
      var m = middle();
      zoomTo(view.scale * step, m[0], m[1]);
    });
    document.getElementById("z-out").addEventListener("click", function () {
      var m = middle();
      zoomTo(view.scale / step, m[0], m[1]);
    });
    document.getElementById("z-reset").addEventListener("click", function () { fit(); });
  }

  function bindPanZoom() {
    var dragging = false;
    var moved = false;
    var from = { x: 0, y: 0 };

    stage.addEventListener("pointerdown", function (e) {
      if (e.target.closest(".task, .prompt-node, .day-node")) return;
      if (pop.contains(e.target)) return;
      dragging = true;
      moved = false;
      from = { x: e.clientX - view.x, y: e.clientY - view.y };
      stage.classList.add("dragging");
      stage.setPointerCapture(e.pointerId);
    });

    stage.addEventListener("pointermove", function (e) {
      if (!dragging) return;
      moved = true;
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
      // Keep whatever is under the pointer under the pointer.
      zoomTo(view.scale * Math.exp(-e.deltaY * 0.0015), mx, my);
    }, { passive: false });

    // A drag that ends on empty canvas should not count as a click, or the
    // popover closes every time you move around.
    stage.addEventListener("click", function (e) {
      if (moved) { moved = false; return; }
      if (pop.contains(e.target)) return;
      if (!e.target.closest(".task, .prompt-node, .day-node")) {
        clear();
        if (openPanel) openPanel(null);
      }
    });

    window.addEventListener("resize", function () {
      if (!selected) fit();
    });
  }

  // ---- reading ------------------------------------------------------------

  // Clicking opens the record and nothing else. The note belongs to hovering,
  // where it answers "what is this" without committing to anything.
  function choose(item) {
    selected = item;
    hide();

    host.querySelectorAll(".chosen").forEach(function (n) {
      n.classList.remove("chosen");
    });
    var node = host.querySelector('[data-id="' + cssEscape(item.id) + '"]');
    if (node) node.classList.add("chosen");

    openReader(item);
  }

  function clear() {
    selected = null;
    hide();
    document.body.classList.remove("reading");
    host.querySelectorAll(".chosen").forEach(function (n) {
      n.classList.remove("chosen");
    });
  }

  // ---- hovering -----------------------------------------------------------

  // Hovering shows a note against the node and lights the whole line back to
  // the spine, so a prompt is always seen as part of the work it belongs to
  // rather than as a loose circle.
  var hovered = null;

  function enter(item) {
    if (hovered === item) return;
    hovered = item;
    litUp(item);
    show(item);
  }

  // dim drops every highlight. Nodes nest, so leaving one is not enough on
  // its own to know nothing is lit any more.
  function dim() {
    ["lit", "lit-hard"].forEach(function (cls) {
      host.querySelectorAll("." + cls).forEach(function (n) {
        n.classList.remove(cls);
      });
    });
  }

  // Nodes nest: a prompt sits inside the task group, which sits inside the
  // day. Leaving one of them is not on its own a reason to put the lights
  // out, since the pointer may have moved onto a child. So a leave only
  // clears when nothing else has claimed the hover in the meantime, and
  // whatever has claimed it is redrawn.
  function leave(item) {
    if (hovered === item) {
      hovered = null;
      dim();
      hide();
      return;
    }
    if (hovered) {
      litUp(hovered);
      show(hovered);
    }
  }

  // litUp marks the node, its parent and its parent's day, plus any link that
  // reaches the day, so work picked up again weeks later lights as one thread.
  function litUp(item) {
    dim();

    // Only what is hovered and the line back to the day it belongs to.
    // Lighting the siblings and the rest of the day as well turned a hover
    // into a wash across half the diagram, which said less than one clear
    // path does.
    var chain = ancestry(item);
    chain.forEach(function (step) { mark(step.id); });

    // A hard task keeps its own colour when lit. Highlighting it green would
    // throw away the one thing the drawing says about it.
    if (hardAt(item)) {
      chain.forEach(function (step) {
        var n = host.querySelector('[data-id="' + cssEscape(step.id) + '"]');
        if (n) n.classList.add("lit-hard");
        var wire = host.querySelector('[data-for="' + cssEscape(step.id) + '"]');
        if (wire) wire.classList.add("lit-hard");
      });
    }
  }

  // hardAt says whether the work under this node was scored as hard, which for
  // a prompt means asking the task it came from.
  function hardAt(item) {
    if (item.kind === "task") return item.hard;
    if (item.kind === "prompt") {
      var task = owner(item);
      return Boolean(task && task.hard);
    }
    return false;
  }

  // mark lights a node and the line that carries it.
  function mark(id) {
    var n = host.querySelector('[data-id="' + cssEscape(id) + '"]');
    if (n) n.classList.add("lit");
    var wire = host.querySelector('[data-for="' + cssEscape(id) + '"]');
    if (wire) wire.classList.add("lit");
  }

  // ancestry returns a node and everything above it, nearest first.
  function ancestry(item) {
    if (item.kind === "day") return [item];
    if (item.kind === "task") return [item, dayOwning(item)].filter(Boolean);
    var task = owner(item);
    return [item, task, task && dayOwning(task)].filter(Boolean);
  }

  function dayOwning(task) {
    for (var d = 0; d < model.days.length; d++) {
      if (model.days[d].tasks.indexOf(task) !== -1) return model.days[d];
    }
    return null;
  }

  // show puts the note together. A prompt is shown with the task it came from
  // and the day it happened on, because a prompt on its own rarely says what
  // it was part of.
  function show(item) {
    pop.textContent = "";

    if (item.kind === "prompt") {
      // The prompt itself, and one line naming the task it came from. Any
      // more than that and the note grows tall enough to have to be placed
      // somewhere other than beside the node, which defeats the point of it.
      pop.appendChild(node("time", "pop-when", when(item.turn.at)));
      pop.appendChild(node("p", "pop-text", clip(item.turn.text || "", 220)));

      var task = owner(item);
      if (task) {
        var from = node("p", "pop-from");
        from.appendChild(node("span", "pop-kind", "in"));
        from.appendChild(document.createTextNode(clip(task.task.label || "", 46)));
        pop.appendChild(from);
      }
    } else if (item.kind === "task") {
      pop.appendChild(node("p", "pop-title", clip(item.task.label || "(unnamed)", 110)));
      pop.appendChild(node("p", "pop-when", figures(item.task.stats)));
    } else {
      pop.appendChild(node("p", "pop-title", item.goal.period || ""));
      pop.appendChild(node("p", "pop-when", figures(item.goal.stats)));
    }

    pop.hidden = false;
    placePop(item);
  }

  function hide() {
    pop.hidden = true;
    pop.at = null;
  }

  // The note sits against the node it belongs to and follows it, so it is
  // never somewhere the eye has to go looking for.
  function placePop(item) {
    if (item) pop.at = item;
    var at = pop.at;
    if (!at || pop.hidden || !at.node) return;

    // Ask the browser where the node actually is, rather than working it out
    // from the pan and zoom. The svg is sized at 100 percent with
    // preserveAspectRatio, so it applies its own scale and centring on top of
    // the transform: reconstructing that by hand was right at the fitted view
    // and drifted further off the more you zoomed in. The element knows.
    var r = at.node.getBoundingClientRect();
    var stageBox = stage.getBoundingClientRect();
    var gap = 10;

    var w = pop.offsetWidth;
    var h = pop.offsetHeight;

    // Beside the node, level with its middle.
    var left = r.right - stageBox.left + gap;
    var top = r.top - stageBox.top + r.height / 2 - h / 2;

    // Flip to the other side when the note would leave the window. Beyond
    // that it does not move: clamping it to the window was what sent it to a
    // corner when a zoomed-in node sat near an edge, which is the opposite of
    // being pegged to the thing it describes. A note half off the screen next
    // to its node is more use than a whole one somewhere else.
    if (left + w > stageBox.width - 4) {
      left = r.left - stageBox.left - gap - w;
    }

    pop.style.left = Math.round(left) + "px";
    pop.style.top = Math.round(top) + "px";
  }

  function openReader(item) {
    var task = item.kind === "prompt" ? owner(item) : item;
    var panel = document.getElementById("reader-body");
    panel.textContent = "";
    document.body.classList.add("reading");

    if (task.kind === "day") {
      readDay(panel, task);
    } else {
      readTask(panel, task, item.kind === "prompt" ? item.index : -1);
    }
  }

  function readDay(panel, day) {
    var head = node("header", "reader-head");
    head.appendChild(node("h2", null, day.goal.period || "a day's work"));
    head.appendChild(node("p", "reader-figures", figures(day.goal.stats)));
    panel.appendChild(head);

    var list = node("ol", "turns");
    day.tasks.forEach(function (task) {
      var li = node("li", "turn");
      li.appendChild(node("p", "said", task.task.label || "(unnamed)"));
      li.appendChild(node("p", "when", figures(task.task.stats)));
      list.appendChild(li);
    });
    panel.appendChild(list);
    document.getElementById("reader").scrollTop = 0;
  }

  function readTask(panel, task, highlight) {
    var stats = task.task.stats;
    var head = node("header", "reader-head");
    head.appendChild(node("h2", null, task.task.label || "(unnamed)"));
    head.appendChild(node("p", "reader-figures", figures(stats)));
    panel.appendChild(head);

    if (stats.churn > 1 && stats.churnFile) {
      panel.appendChild(node("p", "reader-churn",
        "kept coming back to " + baseName(stats.churnFile) +
        " (" + stats.churn + " times)"));
    }

    var wanted = null;
    var list = node("ol", "turns");
    (task.task.turns || []).forEach(function (turn, i) {
      var li = node("li", "turn" + (i === highlight ? " lit" : ""));
      li.appendChild(node("time", "when", when(turn.at)));
      // Prompts are the reader's own words and the reason to look, so they
      // are never trimmed.
      li.appendChild(node("p", "said", turn.text));
      (turn.delegated || []).forEach(function (job) {
        li.appendChild(node("p", "handoff", job.description));
      });
      if (i === highlight) wanted = li;
      list.appendChild(li);
    });
    panel.appendChild(list);

    // Clicking one circle should land on that prompt, not the top of a list
    // of forty.
    document.getElementById("reader").scrollTop = 0;
    if (wanted && wanted.scrollIntoView) {
      wanted.scrollIntoView({ block: "center" });
    }
  }

  // owner finds the task a prompt hangs from.
  function owner(prompt) {
    for (var d = 0; d < model.days.length; d++) {
      var tasks = model.days[d].tasks;
      for (var t = 0; t < tasks.length; t++) {
        if (tasks[t].prompts.indexOf(prompt) !== -1) return tasks[t];
      }
    }
    return null;
  }

  // ---- filtering ----------------------------------------------------------

  // Filters fade rather than remove. Dropping a node would change the shape,
  // and the shape is the thing being looked at.
  function refilter() {
    var needle = filters.file.toLowerCase();
    var anyShown = false;

    model.days.forEach(function (day) {
      var group = host.querySelector('.day[data-id="' + cssEscape(day.id) + '"]');
      if (!group) return;

      var stats = day.goal.stats;
      var dayOut =
        (filters.substantialOnly && (stats.turns || 0) < SUBSTANTIAL) ||
        (filters.from && dayOf(stats.end) < filters.from) ||
        (filters.to && dayOf(stats.start) > filters.to);

      var shown = 0;
      day.tasks.forEach(function (task) {
        var t = host.querySelector('.task[data-id="' + cssEscape(task.id) + '"]');
        if (!t) return;
        var out = dayOut ||
          (needle && !touches(task.task, needle)) ||
          (filters.hardOnly && !task.hard);
        t.classList.toggle("faded", Boolean(out));
        if (!out) shown++;
      });

      group.classList.toggle("faded", dayOut || shown === 0);
      if (shown) anyShown = true;
    });

    document.body.classList.toggle("nothing", !anyShown && filtering());
    marks();
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

  // openPanel is set by bindRail so clicking the canvas can put the card away.
  var openPanel = null;

  function bindRail() {
    var card = document.getElementById("rail-card");
    var tabs = [].slice.call(document.querySelectorAll(".rail-btn[data-panel]"));
    var file = document.getElementById("f-file");
    var from = document.getElementById("f-from");
    var to = document.getElementById("f-to");

    if (graph.goals.length) {
      var first = dayOf(graph.goals[0].stats.start);
      var last = dayOf(graph.goals[graph.goals.length - 1].stats.end);
      [from, to].forEach(function (input) {
        input.min = first;
        input.max = last;
      });
    }

    // One card, one panel showing at a time.
    openPanel = function (name) {
      var showing = false;
      tabs.forEach(function (tab) {
        var mine = tab.getAttribute("data-panel") === name;
        tab.setAttribute("aria-selected", mine ? "true" : "false");
        document.getElementById("p-" + tab.getAttribute("data-panel")).hidden = !mine;
        if (mine) showing = true;
      });
      card.hidden = !showing;
      if (showing && name === "file") file.focus();
    };

    tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        var name = tab.getAttribute("data-panel");
        openPanel(tab.getAttribute("aria-selected") === "true" ? null : name);
      });
    });

    // Every panel has the same way out as the drawer does, rather than making
    // people find the icon they came in by.
    document.querySelectorAll(".rail-back").forEach(function (b) {
      b.addEventListener("click", function () { openPanel(null); });
    });

    // The file search runs as you type, since seeing the shape react is the
    // whole point of it.
    file.addEventListener("input", function () {
      filters.file = file.value.trim();
      refilter();
    });

    // The dates do not, because a half typed year is a filter that hides
    // everything. They wait for Apply.
    document.getElementById("f-when-go").addEventListener("click", function () {
      filters.from = from.value || null;
      filters.to = to.value || null;
      refilter();
    });
    [from, to].forEach(function (input) {
      input.addEventListener("keydown", function (e) {
        if (e.key === "Enter") document.getElementById("f-when-go").click();
      });
    });

    switchFor("f-size", function (on) { filters.substantialOnly = on; });
    switchFor("f-hard", function (on) { filters.hardOnly = on; });

    // A panel's own Clear undoes only that panel, which is what you expect of
    // a button sitting inside it.
    document.getElementById("f-file-clear").addEventListener("click", function () {
      file.value = "";
      filters.file = "";
      refilter();
      file.focus();
    });

    document.getElementById("f-when-clear").addEventListener("click", function () {
      from.value = "";
      to.value = "";
      filters.from = null;
      filters.to = null;
      refilter();
    });

    document.getElementById("f-show-clear").addEventListener("click", function () {
      ["f-size", "f-hard"].forEach(function (id) {
        document.getElementById(id).setAttribute("aria-checked", "false");
      });
      filters.substantialOnly = false;
      filters.hardOnly = false;
      refilter();
    });

  }

  // switchFor wires one of the on-off controls. They are buttons rather than
  // checkboxes because a checkbox in a floating card reads as a form to fill
  // in, and these take effect the moment they are touched.
  function switchFor(id, set) {
    var el = document.getElementById(id);
    el.addEventListener("click", function () {
      var on = el.getAttribute("aria-checked") !== "true";
      el.setAttribute("aria-checked", on ? "true" : "false");
      set(on);
      refilter();
    });
  }

  // marks puts a dot on any tab whose filter is doing something, so a filter
  // left on in a closed panel is never invisible.
  function marks() {
    var live = {
      file: Boolean(filters.file),
      when: Boolean(filters.from || filters.to),
      show: filters.substantialOnly || filters.hardOnly
    };

    document.querySelectorAll(".rail-btn[data-panel]").forEach(function (tab) {
      var on = live[tab.getAttribute("data-panel")];
      tab.classList.toggle("live", Boolean(on));
      tab.querySelector(".dot").hidden = !on;
    });

    // A panel's Clear only appears when it has something to clear.
    ["file", "when", "show"].forEach(function (name) {
      document.getElementById("f-" + name + "-clear").hidden = !live[name];
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

  function dayOf(iso) { return iso ? iso.slice(0, 10) : ""; }

  function baseName(path) {
    var parts = String(path).split(/[\\/]/);
    return parts[parts.length - 1];
  }

  function clip(s, n) {
    s = String(s).replace(/\s+/g, " ").trim();
    return s.length > n ? s.slice(0, n - 1) + "…" : s;
  }

  // Ids are generated as g1.t2.p3, and the dots need escaping in a selector.
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
      [graph.goals.length, "days"],
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
    if (e.key !== "Escape") return;
    clear();
    if (openPanel) openPanel(null);
  });

  chrome();
  if (!graph.goals.length) {
    document.body.classList.add("nothing");
    return;
  }
  draw();
  bindPanZoom();
  bindZoom();
  bindRail();
  document.getElementById("reader-close").addEventListener("click", clear);
})();
