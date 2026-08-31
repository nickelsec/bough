// Renders the graph the server put in the page.
//
// Everything here is presentation. The graph carries no positions, colours or
// sizes on purpose, so the layout is worked out from the data each time rather
// than baked into what the core produces.
(function () {
  "use strict";

  var graph = window.BOUGH;
  if (!graph || !graph.goals) return;

  // A task counts as hard above this. The score behind it is a guess, so it is
  // only ever used to weight a branch, never printed as a figure.
  var HARD = 0.5;

  // Below this a sitting is a loose end rather than a piece of work.
  var SUBSTANTIAL = 5;

  var state = {
    file: "",
    from: null,
    to: null,
    substantialOnly: false,
    hardOnly: false,
    open: new Set()
  };

  var el = {
    tree: document.getElementById("tree"),
    empty: document.getElementById("empty"),
    clear: document.getElementById("filter-clear")
  };

  // ---- small helpers -----------------------------------------------------

  function make(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text != null) node.textContent = text;
    return node;
  }

  function count(n, one, many) {
    return n + " " + (n === 1 ? one : (many || one + "s"));
  }

  function duration(minutes) {
    if (!minutes) return "a moment";
    if (minutes < 60) return minutes + " min";
    var hours = Math.floor(minutes / 60);
    var rest = minutes % 60;
    if (hours < 3 && rest) return hours + "h " + rest + "m";
    return hours + "h";
  }

  function baseName(path) {
    if (!path) return "";
    var parts = path.split(/[\\/]/);
    return parts[parts.length - 1];
  }

  // Fields are left out of the graph when they are zero or empty, so nothing
  // here may assume a key is present.
  function figures(stats) {
    var bits = [count(stats.turns || 0, "prompt")];
    if (stats.edits) bits.push(count(stats.edits, "change"));
    if (stats.errors) bits.push(count(stats.errors, "failure"));
    return bits.join("  ·  ");
  }

  function day(iso) {
    return iso ? iso.slice(0, 10) : "";
  }

  // ---- filtering ---------------------------------------------------------

  function taskMatchesFile(task, needle) {
    if (!needle) return true;
    var files = (task.stats && task.stats.topFiles) || [];
    for (var i = 0; i < files.length; i++) {
      if (files[i].path.toLowerCase().indexOf(needle) !== -1) return true;
    }
    return false;
  }

  function visibleTasks(goal) {
    var needle = state.file.toLowerCase();
    return goal.tasks.filter(function (task) {
      if (!taskMatchesFile(task, needle)) return false;
      if (state.hardOnly && (task.stats.struggle || 0) < HARD) return false;
      return true;
    });
  }

  function visibleGoals() {
    return graph.goals
      .map(function (goal) {
        return { goal: goal, tasks: visibleTasks(goal) };
      })
      .filter(function (row) {
        if (!row.tasks.length) return false;

        var stats = row.goal.stats;
        if (state.substantialOnly && (stats.turns || 0) < SUBSTANTIAL) return false;
        if (state.from && day(stats.end) < state.from) return false;
        if (state.to && day(stats.start) > state.to) return false;
        return true;
      });
  }

  function filtering() {
    return Boolean(state.file || state.from || state.to || state.substantialOnly || state.hardOnly);
  }

  // ---- drawing -----------------------------------------------------------

  function drawTurn(turn) {
    var li = make("li", "turn");
    li.appendChild(make("span", "when", new Date(turn.at).toLocaleString([], {
      month: "short", day: "numeric", hour: "2-digit", minute: "2-digit"
    })));

    var body = make("div");
    body.appendChild(make("p", "said", turn.text));

    (turn.delegated || []).forEach(function (job) {
      body.appendChild(make("span", "handoff", job.description));
    });
    li.appendChild(body);

    if (turn.edits) {
      li.appendChild(make("span", "turn-figures", count(turn.edits, "change")));
    }
    return li;
  }

  function drawTask(task) {
    var li = make("li", "task");
    if ((task.stats.struggle || 0) >= HARD) li.classList.add("hard");

    var open = state.open.has(task.id);

    var head = make("button", "task-head");
    head.type = "button";
    head.setAttribute("aria-expanded", String(open));

    var marker = make("span", "marker", open ? "▾" : "▸");
    head.appendChild(marker);
    head.appendChild(make("span", "task-label", task.label || "(unnamed)"));
    head.appendChild(make("span", "task-figures", figures(task.stats)));
    li.appendChild(head);

    var turns = make("ul", "turns");
    if (!open) turns.classList.add("hidden");
    task.turns.forEach(function (turn) {
      turns.appendChild(drawTurn(turn));
    });
    li.appendChild(turns);

    head.addEventListener("click", function () {
      var nowOpen = !state.open.has(task.id);
      if (nowOpen) state.open.add(task.id);
      else state.open.delete(task.id);

      turns.classList.toggle("hidden", !nowOpen);
      marker.textContent = nowOpen ? "▾" : "▸";
      head.setAttribute("aria-expanded", String(nowOpen));
    });

    return li;
  }

  function drawGoal(row) {
    var goal = row.goal;
    var section = make("section", "sitting");

    var head = make("div", "sitting-head");
    head.appendChild(make("span", "period", goal.period || ""));
    head.appendChild(make("span", "sitting-label", goal.label || "(unnamed)"));

    var stats = goal.stats;
    var summary = figures(stats);
    if (stats.activeMinutes) summary += "  ·  " + duration(stats.activeMinutes);
    head.appendChild(make("span", "sitting-figures", summary));
    section.appendChild(head);

    // Returning to one file over and over is the clearest sign of a struggle,
    // and unlike the score it is a plain count.
    if (stats.churn > 1 && stats.churnFile) {
      section.appendChild(make("span", "churn",
        "kept coming back to " + baseName(stats.churnFile) + " (" + stats.churn + " times)"));
    }

    var list = make("ul", "tasks");
    row.tasks.forEach(function (task) {
      list.appendChild(drawTask(task));
    });
    section.appendChild(list);

    // Links are absent from the graph entirely when there are none.
    var back = (graph.links || []).filter(function (link) {
      return link.to === goal.id;
    });
    back.forEach(function (link) {
      var from = graph.goals.find(function (g) { return g.id === link.from; });
      if (!from) return;
      var names = link.files.slice(0, 3).map(baseName).join(", ");
      var more = link.files.length > 3 ? ", and " + (link.files.length - 3) + " more" : "";
      section.appendChild(make("p", "came-back",
        "picked up from " + from.period + ": " + names + more));
    });

    return section;
  }

  function draw() {
    var rows = visibleGoals();
    el.tree.textContent = "";

    rows.forEach(function (row) {
      el.tree.appendChild(drawGoal(row));
    });

    var nothing = rows.length === 0;
    el.empty.hidden = !nothing;
    if (nothing) {
      el.empty.textContent = filtering()
        ? "Nothing matches those filters."
        : "No work found in this project.";
    }
    el.clear.hidden = !filtering();
  }

  // ---- the parts that do not change --------------------------------------

  function header() {
    document.getElementById("project-name").textContent = graph.project.name;
    document.getElementById("project-path").textContent = graph.project.path;

    var totals = graph.totals;
    var pairs = [
      ["prompts", totals.turns || 0],
      ["sittings", graph.goals.length],
      ["changes", totals.edits || 0],
      ["files", totals.files || 0],
      ["at the keyboard", duration(totals.activeMinutes)]
    ];

    var dl = document.getElementById("totals");
    pairs.forEach(function (pair) {
      var wrap = make("div");
      wrap.appendChild(make("dt", null, pair[0]));
      wrap.appendChild(make("dd", null, String(pair[1])));
      dl.appendChild(wrap);
    });

    var span = "";
    if (graph.goals.length) {
      span = graph.goals[0].period + " to " + graph.goals[graph.goals.length - 1].period;
    }
    document.getElementById("footprint").textContent = span;
  }

  function bindFilters() {
    var file = document.getElementById("filter-file");
    var from = document.getElementById("filter-from");
    var to = document.getElementById("filter-to");
    var size = document.getElementById("filter-size");
    var hard = document.getElementById("filter-struggle");

    // Bound the date pickers to the history that exists, so there is nothing
    // to scrub through that could never match.
    if (graph.goals.length) {
      var first = day(graph.goals[0].stats.start);
      var last = day(graph.goals[graph.goals.length - 1].stats.end);
      [from, to].forEach(function (input) {
        input.min = first;
        input.max = last;
      });
    }

    file.addEventListener("input", function () { state.file = file.value.trim(); draw(); });
    from.addEventListener("change", function () { state.from = from.value || null; draw(); });
    to.addEventListener("change", function () { state.to = to.value || null; draw(); });
    size.addEventListener("change", function () { state.substantialOnly = size.checked; draw(); });
    hard.addEventListener("change", function () { state.hardOnly = hard.checked; draw(); });

    el.clear.addEventListener("click", function () {
      file.value = ""; from.value = ""; to.value = "";
      size.checked = false; hard.checked = false;
      state.file = ""; state.from = null; state.to = null;
      state.substantialOnly = false; state.hardOnly = false;
      draw();
    });
  }

  header();
  bindFilters();
  draw();
})();
