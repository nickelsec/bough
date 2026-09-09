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

  // The floor for the mark on a task that ended in a commit, against the zoom.
  // Small on purpose: it says the work landed, and it must not read as a score.
  var SHIPPED_R = 1.7;

  var sized = [];

  // Two scales worth naming.
  //
  // whole shows every day at once. On a short history that is the right first
  // sight of it. On a long one it is a thin line: the diagram grows sideways
  // with each day worked and never grows taller, so a hundred days is a ribbon
  // around twenty nine times wider than it is tall, and fitting that to a
  // window draws the day squares at under three pixels.
  //
  // home is the scale the nodes are legible at, and it is where the view
  // opens. Where the whole diagram fits at that scale the two are the same and
  // nothing changes; where it does not, time becomes something you travel
  // along rather than something squeezed onto one screen.
  var whole = 1;
  var home = 1;

  // A day square is 36 units. Legible means about this many pixels for one,
  // which keeps a prompt circle near eleven and comfortably clickable.
  var READABLE = 32 / 36;

  // The zoom range, as multiples of home rather than as absolute scales. An
  // absolute cap means something different on every project: at three it was
  // 273% of the opening view on a short history and 4092% on a long one, which
  // is why the readout could say 499% while the diagram was still a line.
  var RANGE = { min: 0.25, max: 4 };

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
    reset();
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

    // Work that ended in a commit gets a small filled mark at the corner.
    // Deliberately quiet: plenty of real work never commits, and a task that
    // did not is not a worse task. It marks what landed, not what was good.
    if (task.shipped) {
      var mark = el("circle", { class: "shipped-mark", r: SHIPPED_R });
      task.mark = mark;
      g.appendChild(mark);
    }

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

      // The shipped mark rides the corner of the square it belongs to, so it
      // has to move whenever the square is resized against the zoom. It sits
      // inside the corner rather than on it, which reads as part of the node
      // instead of something stuck to its edge.
      if (item.at.mark) {
        // A fixed share of the square, so it reads the same on every task and
        // sits inside the corner rather than on it. Held to a floor against
        // the zoom the way the nodes themselves are, or it disappears at the
        // scale that fits a wide project on screen.
        //
        // These two numbers were worked out rather than tried: at a ratio of
        // 0.13 and a pad of 0.2 the dot clears the inside of the stroke by a
        // full unit even on the smallest task drawn.
        var r = Math.max(on * 0.13, SHIPPED_R / view.scale);
        var inset = r + on * 0.2;
        item.at.mark.setAttribute("r", r);
        item.at.mark.setAttribute("cx", item.at.x + half - inset);
        item.at.mark.setAttribute("cy", item.at.y - half + inset);
      }
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

  // content is the box the drawing actually occupies.
  //
  // The layout reserves a wide margin around the diagram so there is somewhere
  // to pan to, and on every project measured that leaves a 300px band of empty
  // canvas. Fitting to the canvas rather than to the drawing scales for space
  // nothing is drawn in: a short history filled barely half the height it was
  // given, so it opened at about half the size it could have and sat off
  // centre, which reads as the view having failed to fit at all.
  function content() {
    var x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    function seen(x, y, r) {
      x0 = Math.min(x0, x - r); x1 = Math.max(x1, x + r);
      y0 = Math.min(y0, y - r); y1 = Math.max(y1, y + r);
    }
    model.days.forEach(function (day) {
      seen(day.x, day.y, day.size / 2);
      day.tasks.forEach(function (task) {
        seen(task.x, task.y, task.size / 2);
        task.prompts.forEach(function (p) { seen(p.x, p.y, p.r); });
      });
    });
    // Nothing was drawn, so fall back to the canvas rather than to infinities.
    if (!isFinite(x0)) return { x: 0, y: 0, width: model.width, height: model.height };

    // The spine is deliberately left out of the horizontal reckoning. It runs
    // half a margin past the last day at the arrow end and only a few pixels
    // before the first at the other, so a box drawn round it is lopsided by
    // about fifty pixels. Centring that box puts the work itself off to one
    // side, which is the thing being fixed. The nodes are the content; the
    // line they sit on is a backdrop and may run off the view.
    //
    // Vertically it matters, since the spine is the axis every day hangs from
    // and a diagram with one row of tasks would otherwise be measured from
    // the tasks alone and sit with the spine against an edge.
    if (model.spine) {
      y0 = Math.min(y0, model.spine.y - 13);
      y1 = Math.max(y1, model.spine.y + 13);
    }

    // Room under the day squares for their dates, which sit outside the
    // shapes and would otherwise be cropped at the bottom edge.
    var labels = 30;
    return { x: x0, y: y0, width: x1 - x0, height: y1 - y0 + labels };
  }

  // room is the drawable area, less a margin, and never smaller than a pixel.
  // A window narrower than its own margins would otherwise give a scale of
  // zero or less, and an empty stage no amount of panning recovers.
  function room() {
    var box = stage.getBoundingClientRect();
    var margin = box.width < 560 ? 16 : 40;
    return {
      w: Math.max(box.width - margin * 2, 1),
      h: Math.max(box.height - margin * 2, 1),
      box: box
    };
  }

  // wholeScale is the largest scale that still shows every day at once.
  function wholeScale(seen, r) {
    var s = Math.min(1.1, r.h / seen.height, r.w / seen.width);

    // The spine runs past the nodes at both ends, further at the arrow. It is
    // not centred on, or the work sits off to one side, but it still has to
    // fit, or the arrow is clipped by the edge of the window.
    if (model.spine) {
      var mid = seen.x + seen.width / 2;
      var reach = Math.max(mid - (model.spine.x1 - 13), model.spine.x2 + 13 - mid) * 2;
      if (reach > 0) s = Math.min(s, r.w / reach);
    }
    return s > 0 ? s : 1;
  }

  // homeScale is where the view opens: legible, but never larger than showing
  // the whole thing, since blowing up a two day history helps nobody.
  function homeScale(seen, r) {
    var all = wholeScale(seen, r);
    if (all >= READABLE) return all;

    // A diagram that very nearly fits is better shown whole. Measured on an
    // eleven day history: opening at the legible scale gained seventeen
    // percent on the nodes and cost seven percent off the right hand edge,
    // which trades a complete picture for almost nothing. Below three
    // quarters legible the nodes are small enough that panning is the better
    // bargain.
    if (all >= READABLE * 0.75) return all;

    // Legible, but still bounded by the height. A ribbon may be scrolled
    // sideways; one taller than the window has nowhere to go.
    return Math.min(READABLE, Math.max(all, r.h / seen.height));
  }

  // frame puts the view at a scale, centred, or against the end of a diagram
  // too wide to show at once.
  //
  // Time runs left to right, so the end is the most recent work. That is what
  // somebody opening this came to see, and the spine runs back into the
  // history behind it.
  function frame(scale, atEnd) {
    var seen = content();
    var r = room();
    view.scale = scale;

    var wide = seen.width * scale > r.w;
    if (wide && atEnd) {
      var margin = (r.box.width - r.w) / 2;
      view.x = r.box.width - margin - (seen.x + seen.width) * scale;
    } else {
      view.x = (r.box.width - seen.width * scale) / 2 - seen.x * scale;
    }
    view.y = (r.box.height - seen.height * scale) / 2 - seen.y * scale;
    apply();
  }

  // measure works out both anchors for the current window. Called on load and
  // whenever the window changes, since either can move.
  function measure() {
    if (!model || !model.height) return false;
    var seen = content();
    var r = room();
    whole = wholeScale(seen, r);
    home = homeScale(seen, r);
    return true;
  }

  // reset returns to the opening view, which is the one meant to be worked in.
  function reset() {
    if (!measure()) return;
    frame(home, true);
  }

  // fit shows every day at once, however small that makes them. On a short
  // history it is the same view as reset; on a long one it is the shape of the
  // whole thing rather than a piece of it.
  function fit() {
    if (!measure()) return;
    frame(whole, false);
  }

  // zoomTo changes the scale about a point, which is the pointer for a wheel
  // and the middle of the view for a button.
  function zoomTo(next, ax, ay) {
    // The range is relative to the opening scale, so four hundred percent
    // means the same thing on a two day history and a two hundred day one.
    // The whole view is always reachable, however far out that is, or a long
    // project could never be seen entire.
    var low = Math.min(home * RANGE.min, whole);
    next = Math.min(home * RANGE.max, Math.max(low, next));
    view.x = ax - (ax - view.x) * (next / view.scale);
    view.y = ay - (ay - view.y) * (next / view.scale);
    view.scale = next;
    apply();
  }

  function readout() {
    var now = document.getElementById("z-now");
    if (!now) return;
    // Against home, not against the whole. The number now means the same on
    // every project: a hundred percent is the view it opened at.
    now.textContent = Math.round((view.scale / home) * 100) + "%";

    var low = Math.min(home * RANGE.min, whole);
    document.getElementById("z-in").disabled = view.scale >= home * RANGE.max - 0.001;
    document.getElementById("z-out").disabled = view.scale <= low + 0.001;

    // The button that shows everything is only worth offering when there is
    // more than the screen already holds.
    var all = document.getElementById("z-all");
    if (all) {
      all.hidden = whole >= home - 0.001;
      all.setAttribute("aria-pressed", String(Math.abs(view.scale - whole) < 0.001));
    }
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
    // The readout returns to the view it opened at, which is the one meant to
    // be worked in. Showing every day at once is its own button, and only
    // appears when there is more than the screen already holds.
    document.getElementById("z-reset").addEventListener("click", function () { reset(); });
    var all = document.getElementById("z-all");
    if (all) all.addEventListener("click", function () { fit(); });
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
      if (!selected) reset();
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
      if (item.shipped) {
        var hashes = shas(item.task.stats);
        if (hashes) pop.appendChild(node("p", "pop-sha", hashes));
      }
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

    commitList(panel, day.goal.stats.commits);

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
      // The count says the file was returned to. The lines say whether that
      // meant a typo or a rewrite, which the count alone cannot.
      var much = stats.churn + " times";
      if (stats.lineChurn) much += ", " + stats.lineChurn + " lines";
      panel.appendChild(node("p", "reader-churn",
        "kept coming back to " + baseName(stats.churnFile) + " (" + much + ")"));
    }

    // What share of the project this work took. A proportion rather than a
    // count, since the absolute number means nothing without the whole.
    if (stats.tokens && graph.totals.tokens) {
      var mine = tokensOf(stats.tokens);
      var whole = tokensOf(graph.totals.tokens);
      if (whole > 0 && mine > 0) {
        var pct = 100 * mine / whole;
        panel.appendChild(node("p", "reader-cost",
          (pct < 1 ? "under 1" : "~" + Math.round(pct)) + "% of this project's tokens"));
      }
    }

    commitList(panel, stats.commits);

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
      // A commit sits under the prompt that produced it, so the record reads
      // as what was asked for and what came of it.
      (turn.committed || []).forEach(function (c) {
        li.appendChild(commitRow(c));
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

  // commitList heads the record with everything the work committed.
  //
  // The same commits appear again further down against the prompts that made
  // them. That repetition is deliberate: this answers "what did this ship",
  // which people want without reading, and the ones below answer "what caused
  // it", which only makes sense in place.
  function commitList(panel, commits) {
    var all = commits || [];
    if (!all.length) return;

    var box = node("section", "commits");
    box.appendChild(node("h3", "commits-head",
      all.length === 1 ? "1 commit" : all.length + " commits"));

    var list = node("ol", "commit-list");
    all.forEach(function (c) {
      list.appendChild(commitRow(c, true));
    });
    box.appendChild(list);
    panel.appendChild(box);
  }

  // commitRow draws one commit. As a list item inside the summary, and as a
  // plain line where it hangs under a prompt.
  function commitRow(c, asItem) {
    var row = node(asItem ? "li" : "p", "commit");

    if (c.sha) {
      row.appendChild(node("code", "commit-sha", c.sha));
    }
    // Without a hash there is still something true to say: it happened. That
    // is the case for a commit made with git's quiet flag on a project whose
    // repository could not be read.
    row.appendChild(node("span", "commit-subject",
      c.subject || (c.kind === "amended" ? "amended a commit" : "committed")));

    if (c.added || c.removed) {
      var churn = node("span", "commit-churn");
      if (c.added) churn.appendChild(node("b", "plus", "+" + c.added));
      if (c.removed) churn.appendChild(node("b", "minus", "−" + c.removed));
      row.appendChild(churn);
    }
    return row;
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
    var commits = stats.commits || [];
    if (commits.length) bits.push(count(commits.length, "commit"));
    return bits.join("  ·  ");
  }

  // The hashes themselves, which are the one thing on the page a reader can
  // check against their own repository.
  //
  // Not every commit has one. Claude Code reads the hash out of what git
  // printed, and a commit made quietly prints nothing, so the commit is known
  // to have happened while its hash is not.
  // Hashes only in the note, and no messages.
  //
  // The note is a glance, not a read. Messages turned it into a wall of text
  // that had to be truncated twice over, and anyone who wants to know what a
  // commit said can open the record, where they are listed in full beside the
  // prompts that produced them.
  function shas(stats) {
    var all = [];
    (stats.commits || []).forEach(function (c) {
      if (c.sha) all.push(c.sha);
    });
    return all.join("  ");
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

    // Two rows. The first is how much work there was, which is what someone
    // wants at a glance. The second is what it produced and what it cost,
    // which is a different question and was making the bar long enough to
    // wrap on a laptop.
    var main = [
      [t.turns || 0, "prompts"],
      [graph.goals.length, "days"],
      [t.edits || 0, "changes"],
      [t.files || 0, "files"],
      [duration(t.activeMinutes), "at the keyboard"]
    ];

    var rest = [];
    var made = (t.commits || []).length;
    if (made) rest.push([made, made === 1 ? "commit" : "commits", true]);
    // As a ratio rather than a share of the total. Every project measured came
    // out between 99.4 and 99.9 per cent context, so a percentage says the
    // same about all of them and rounds to a hundred, which reads as though
    // nothing was written. The multiple is what varies.
    if (t.tokens && t.tokens.output) {
      var tk = t.tokens;
      rest.push([big(tk.output), "written"]);
      rest.push([big(tk.cacheRead || 0), "re-read"]);
      if (tk.cacheRead) {
        rest.push([Math.round(tk.cacheRead / tk.output) + "x", "more context than output"]);
      }
    }
    if (t.models) rest.push([modelShare(t.models), ""]);

    fill(document.getElementById("foot-main"), main);
    fill(document.getElementById("foot-rest"), rest);

    // The first row is itself the control. With nothing behind it there is
    // nothing to open, so it goes back to being a plain row of figures rather
    // than a button that does nothing.
    var more = document.getElementById("foot-more");
    if (!rest.length) {
      more.removeAttribute("aria-expanded");
      more.removeAttribute("aria-controls");
      more.classList.add("inert");
      return;
    }
    // Left open if it was left open. Someone who wants the token figures
    // usually wants them again, and reopening this every visit is a small
    // annoyance that adds up. Wrapped because a browser told to block site
    // data throws on the attempt rather than returning nothing.
    var KEY = "bough.foot";
    var want = false;
    try {
      want = localStorage.getItem(KEY) === "open";
    } catch (e) { /* no storage, so it starts closed */ }
    show(want);

    more.addEventListener("click", function () {
      var open = more.getAttribute("aria-expanded") === "true";
      show(!open);
      try {
        localStorage.setItem(KEY, open ? "shut" : "open");
      } catch (e) { /* nothing to remember it with */ }
    });

    function show(open) {
      more.setAttribute("aria-expanded", open ? "true" : "false");
      document.getElementById("foot-rest").hidden = !open;
    }
  }

  // fill writes one row of figures.
  function fill(into, rows) {
    rows.forEach(function (pair) {
      var span = node("span");
      if (pair[0] !== "") span.appendChild(node("b", null, String(pair[0])));
      // The gap between a number and its word is set in the stylesheet, so
      // the word is added without one of its own.
      if (pair[1]) span.appendChild(document.createTextNode(pair[1]));
      if (pair[2]) span.appendChild(countNote());
      into.appendChild(span);
    });

  }

  // tokensOf sums the four counts into one number.
  function tokensOf(t) {
    return (t.input || 0) + (t.output || 0) + (t.cacheRead || 0) + (t.cacheWrite || 0);
  }

  // big shortens a token count to something readable. Nobody needs the last
  // six digits of a billion.
  function big(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1) + "B";
    if (n >= 1e6) return (n / 1e6).toFixed(1) + "M";
    if (n >= 1e3) return Math.round(n / 1e3) + "k";
    return String(n);
  }

  // modelShare names the models that did the work, most first.
  //
  // Almost every project uses one, and naming it is enough. A project that
  // changed model partway through is the interesting case, and then the split
  // is worth seeing.
  function modelShare(models) {
    var names = Object.keys(models);
    if (!names.length) return "";
    names.sort(function (a, b) { return models[b] - models[a]; });
    if (names.length === 1) return names[0];
    var all = 0;
    names.forEach(function (n) { all += models[n]; });
    // The largest share takes what the rounding left over, so the parts add
    // up to a hundred rather than to ninety nine.
    var parts = [], rest = 100;
    for (var i = names.length - 1; i > 0; i--) {
      var pct = Math.round(100 * models[names[i]] / all);
      parts[i] = names[i] + " " + pct + "%";
      rest -= pct;
    }
    parts[0] = names[0] + " " + rest + "%";
    return parts.join("  ·  ");
  }

  // queryMark draws the small circled question the note hangs off.
  //
  // Drawn rather than typed. A "?" in the page's mono face sits on the text
  // baseline and reads as a character someone left behind; a circle at the
  // cap height reads as something to hover. It takes its colour from the
  // button, so the one hover rule below moves both.
  function queryMark() {
    var ns = "http://www.w3.org/2000/svg";
    var svg = document.createElementNS(ns, "svg");
    svg.setAttribute("viewBox", "0 0 16 16");
    svg.setAttribute("aria-hidden", "true");
    svg.setAttribute("focusable", "false");

    var ring = document.createElementNS(ns, "circle");
    ring.setAttribute("cx", "8");
    ring.setAttribute("cy", "8");
    ring.setAttribute("r", "6.6");
    ring.setAttribute("fill", "none");
    ring.setAttribute("stroke", "currentColor");
    ring.setAttribute("stroke-width", "1.3");
    svg.appendChild(ring);

    // The hook of the question, then its dot.
    //
    // Both sit 0.3 higher than the arithmetic centre. The glyph runs from the
    // top of the hook's arc to the bottom of the dot, and that span centres
    // below the ring's own centre unless it is lifted, which reads as a
    // question mark sitting low in its circle.
    var hook = document.createElementNS(ns, "path");
    hook.setAttribute("d", "M6.1 5.8a1.95 1.95 0 1 1 2.6 1.85c-.45.18-.7.5-.7.95v.5");
    hook.setAttribute("fill", "none");
    hook.setAttribute("stroke", "currentColor");
    hook.setAttribute("stroke-width", "1.3");
    hook.setAttribute("stroke-linecap", "round");
    svg.appendChild(hook);

    var dot = document.createElementNS(ns, "circle");
    dot.setAttribute("cx", "8");
    dot.setAttribute("cy", "11.3");
    dot.setAttribute("r", "0.85");
    dot.setAttribute("fill", "currentColor");
    svg.appendChild(dot);
    return svg;
  }

  // countNote explains why this number and the one on a forge may differ.
  //
  // bough counts what the agent did; git log holds what survived. Usually the
  // same number, not always, and a reader who spots the gap should find the
  // reason here rather than conclude the tool is wrong. Three lines is the
  // whole of it: anything longer stops being read.
  function countNote() {
    var mark = node("button", "note");
    mark.type = "button";
    mark.appendChild(queryMark());
    mark.setAttribute("aria-label", "why this may differ from GitHub");
    mark.title = [
      "May differ from GitHub on these cases:",
      "• Commits you made by hand",
      "• Amends, counted where they happened",
      "• Rebases, which drop commits"
    ].join("\n");
    return mark;
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
