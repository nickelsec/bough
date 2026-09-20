// Works out where every node and line goes.
//
// Nothing here draws anything. It takes the graph and returns nodes and edges,
// which keeps it testable: two nodes on top of each other or a runaway canvas
// fails an assertion instead of being noticed by eye later.
//
// The arrangement is the one you would draw on paper. A spine runs left to
// right through time, with a square for every day worked. Each day carries its
// tasks above and below it, and each task carries a circle for every prompt.
// Straight lines join a node to its parent and to nothing else.
//
// Every number comes from the data or from a constant below, and nothing is
// random, so the same history always draws the same way and a screenshot of it
// is stable.
(function (root) {
  "use strict";

  // Two forms of the same diagram. The overview packs it in so the whole thing
  // fits a window; the reading form gives each node the room its label needs.
  var FORM = {
    overview: { dayGap: 150, taskGap: 132, stem: 92, promptGap: 24, promptRun: 5, rows: 2, spread: 168 },
    reading: { dayGap: 240, taskGap: 178, stem: 130, promptGap: 32, promptRun: 5, rows: 2, spread: 250 }
  };

  // Node sizes. A day is a square, a task is a smaller square, a prompt is a
  // circle. The day square grows a little with the work in it, but stays
  // inside a range so the row of them still reads as a row.
  var DAY = { min: 26, max: 46 };
  var TASK = { min: 14, max: 26 };
  var PROMPT_R = 6;

  // The field behind a task, carrying what it cost.
  //
  // One scale across the whole drawing, so two tasks charged the same are
  // drawn the same. Tying it to each task's own prompt spread was tried and
  // is worse than what it fixed: it made the circle partly a count of
  // prompts, and identical spend came out six times the size on a task with
  // thirty prompts as on one with a single prompt.
  //
  // The ceiling is a guard rather than the encoding, applied per task below:
  // a field may reach past its prompts, since it is drawn behind everything
  // and is barely there, but it may not dwarf the work it sits behind.
  var HALO = { min: 12, max: 46 };

  var MARGIN = 120;
  var SPINE_Y = 0; // filled in once the tallest column is known

  // A task at or above this counts as hard. The score behind it is a guess, so
  // it only ever changes the colour and never appears as a figure.
  var HARD = 0.5;

  // A day square is 36 units. Legible means about this many pixels for one,
  // which keeps a prompt circle near eleven and comfortably clickable.
  var READABLE = 32 / 36;

  // The other end of the same question. A ceiling on how big a node may be
  // drawn, not on the scale: an absolute cap means something different on a
  // small project, where the whole diagram came to forty pixels across in the
  // middle of a window fourteen hundred wide, geometrically centred and still
  // reading as lost.
  var COMFORTABLE = 72 / 36;

  // wholeScale is the largest scale that still shows every day at once.
  //
  // seen is the box the drawing occupies, r the room available, spine the
  // model's spine or null. It lives here rather than in the page because it is
  // arithmetic over the layout, and the check that guards it can only load
  // what this file exports: the copy that used to sit in the checker asserted
  // against its own restatement of these lines.
  function wholeScale(seen, r, spine) {
    var s = Math.min(COMFORTABLE, r.h / seen.height, r.w / seen.width);

    // The spine runs past the nodes at both ends, further at the arrow. It is
    // not centred on, or the work sits off to one side, but it still has to
    // fit, or the arrow is clipped by the edge of the window.
    if (spine) {
      var mid = seen.x + seen.width / 2;
      var reach = Math.max(mid - (spine.x1 - 13), spine.x2 + 13 - mid) * 2;
      if (reach > 0) s = Math.min(s, r.w / reach);
    }
    return s > 0 ? s : 1;
  }

  // homeScale is where the view opens: legible, but never larger than showing
  // the whole thing, since blowing up a two day history helps nobody.
  function homeScale(seen, r, spine) {
    var all = wholeScale(seen, r, spine);
    if (all >= READABLE) return all;

    // Legible, but still bounded by the height. A ribbon may be scrolled
    // sideways; one taller than the window has nowhere to go.
    var legible = Math.min(READABLE, Math.max(all, r.h / seen.height));

    // Showing the whole diagram is worth having, but never at the cost of the
    // nodes being smaller than they need to be. Where the whole thing fits at
    // the legible scale it is already being shown; where it does not, opening
    // whole means shrinking below legible, and that is the thing being fixed.
    //
    // An earlier version took the whole view whenever it came within a fixed
    // fraction of legible. That fraction ignored how much bigger the nodes
    // would actually be, and it produced a diagram that shrank as the window
    // grew: a twelve day history opened at thirty two pixels on a 1440 screen
    // and thirty on a 1920 one, because the wider screen brought the whole
    // view inside the threshold. A larger window must never give smaller
    // nodes.
    return Math.max(all, legible);
  }

  function clamp(v, lo, hi) {
    return v < lo ? lo : v > hi ? hi : v;
  }

  // spent is what a piece of work was charged for, all four figures added.
  //
  // The four are disjoint, so adding them counts nothing twice. Cache read is
  // most of it on every history measured, which means this tracks how long the
  // conversation grew as much as how hard the work was. It is the figure the
  // agents themselves report and the one other tools show, so it is the one
  // shown here.
  function spent(stats) {
    var t = stats && stats.tokens;
    if (!t) return 0;
    return (t.input || 0) + (t.output || 0) + (t.cacheRead || 0) + (t.cacheWrite || 0);
  }

  // field maps what a task cost onto the radius drawn behind it.
  //
  // Not size() above, which square-roots. That is right for a node, where one
  // busy day would otherwise flatten every other to the minimum, but it is
  // wrong here. Spend runs over four orders of magnitude on a real project and
  // the square root pulls the middle of that into a band a few pixels wide: on
  // this history it put half of them between 20 and 28, which the eye reads as
  // all the same. An exponent nearer one keeps the quiet work small and lets
  // the expensive work actually show, which is the only thing this is for.
  function field(value, most, ring) {
    if (!most) return 0;
    var at = Math.pow(clamp(value / most, 0, 1), 0.85);
    var r = HALO.min + (HALO.max - HALO.min) * at;
    // Never past this task's own ring of prompts. Without it a project of
    // three tasks, which opens zoomed well in, drew a circle three times the
    // width of the task and swallowed every prompt hanging off it. Reaching
    // the ring is fine and is what it looks like on a dense project; going
    // beyond it is what stops reading as ground and starts hiding the work.
    //
    // The clamp only bites on a small or sparse project, where the spread it
    // costs is spread there was no room to show anyway.
    return Math.round(Math.min(r, ring));
  }

  // size maps a count onto a node size. The square root matters: with linear
  // scaling one busy day flattens every other node to the minimum.
  function size(value, most, range) {
    if (!most) return range.min;
    return Math.round(
      range.min + (range.max - range.min) * Math.sqrt(clamp(value / most, 0, 1))
    );
  }

  function days(a, b) {
    return (new Date(b) - new Date(a)) / 86400000;
  }

  // build returns everything needed to draw one diagram.
  function build(graph, mode) {
    var form = FORM[mode] || FORM.overview;
    var goals = graph.goals || [];
    if (!goals.length) {
      return { width: 0, height: 0, spineY: 0, spine: null, days: [], links: [] };
    }

    var mostDayEdits = 0;
    var mostTaskEdits = 0;
    var mostTaskTokens = 0;
    goals.forEach(function (goal) {
      mostDayEdits = Math.max(mostDayEdits, goal.stats.edits || 0);
      goal.tasks.forEach(function (task) {
        mostTaskEdits = Math.max(mostTaskEdits, task.stats.edits || 0);
        mostTaskTokens = Math.max(mostTaskTokens, spent(task.stats));
      });
    });

    // Lay each day out around a spine at y = 0 first, then shift the whole
    // thing down once we know how far the tallest column reached upward.
    var laid = [];
    var x = MARGIN;
    var previousEnd = null;
    var above = 0;
    var below = 0;

    goals.forEach(function (goal) {
      // A pure time axis collides, because two sittings on the same day land
      // on top of each other. Time decides the pause, content decides the
      // room, and the column always gets at least what it needs.
      if (previousEnd) {
        var apart = Math.max(0, days(previousEnd, goal.stats.start));
        x += form.dayGap * (0.55 + 0.45 * Math.sqrt(clamp(apart / 3, 0, 1)));
      }

      var day = {
        id: goal.id,
        goal: goal,
        kind: "day",
        x: Math.round(x),
        y: SPINE_Y,
        size: size(goal.stats.edits || 0, mostDayEdits, DAY),
        tasks: []
      };

      // Tasks alternate above and below the spine, so a busy day grows in both
      // directions rather than into a tall stack on one side.
      var up = 0;
      var down = 0;
      var reach = 0;

      goal.tasks.forEach(function (task, i) {
        var upward = i % 2 === 0;
        var rank = upward ? up++ : down++;
        var side = upward ? -1 : 1;
        var box = size(task.stats.edits || 0, mostTaskEdits, TASK);
        var cost = spent(task.stats);
        // How far this task's own prompts sit from it, which is what the
        // field behind it is held inside. A task with no prompts still has a
        // box to sit behind, so the first ring stands in for it.
        //
        // Not named reach: the day's own reach is tracked in this function
        // too, and a second var of that name silently overwrote it on every
        // task, which sent whole days off the edge of the canvas.
        var rings = Math.max(1, Math.ceil((task.turns || []).length / form.promptRun));
        var ring = form.promptGap * rings;

        // A busy day spreads sideways once it has stacked a couple of rows.
        // Without this one ten-task day sets the height of the whole canvas
        // and every quiet day around it is drawn tiny to fit.
        var row = rank % form.rows;
        var column = Math.floor(rank / form.rows);

        var node = {
          id: task.id,
          task: task,
          kind: "task",
          hard: (task.stats.struggle || 0) >= HARD,
          // Whether the work landed. Unlike hard, this is not a guess.
          shipped: (task.stats.commits || []).length > 0,
          side: side,
          x: day.x + column * form.spread,
          y: SPINE_Y + side * (form.stem + row * form.taskGap),
          size: box,
          // How much this piece of work was charged for, as a radius. Zero
          // when the history predates the agent recording it, and nothing is
          // drawn for that rather than a smallest ring, which would say the
          // work was cheap when the truth is that nobody knows.
          halo: cost > 0 ? field(cost, mostTaskTokens, ring) : 0,
          prompts: []
        };

        // Prompts run outward from their task in a short column, wrapping into
        // a second and third file rather than growing without limit.
        var turns = task.turns || [];
        for (var p = 0; p < turns.length; p++) {
          var file = Math.floor(p / form.promptRun);
          var place = p % form.promptRun;
          var run = Math.min(form.promptRun, turns.length - file * form.promptRun);
          node.prompts.push({
            id: task.id + ".p" + (p + 1),
            turn: turns[p],
            index: p,
            kind: "prompt",
            // Centre each file of prompts on the task, so a task with two
            // prompts is not lopsided against one with five.
            x: node.x + (place - (run - 1) / 2) * form.promptGap,
            y: node.y + side * (form.promptGap + file * form.promptGap),
            r: PROMPT_R
          });
        }

        var depth = Math.abs(node.y - SPINE_Y) +
          Math.ceil(turns.length / form.promptRun) * form.promptGap + PROMPT_R + 30;
        if (upward) above = Math.max(above, depth);
        else below = Math.max(below, depth);

        // A spread day, or a wide file of prompts, sticks out past the column
        // the day started in, and the next day has to clear it.
        var half = ((Math.min(form.promptRun, turns.length) - 1) / 2) * form.promptGap;
        reach = Math.max(reach, column * form.spread + half + PROMPT_R);

        day.tasks.push(node);
      });

      day.reach = reach;
      laid.push(day);
      x += reach;
      previousEnd = goal.stats.end;
    });

    // Now that the extremes are known, drop everything so the topmost node
    // sits just under the margin.
    var shift = above + MARGIN;
    laid.forEach(function (day) {
      day.y += shift;
      day.tasks.forEach(function (task) {
        task.y += shift;
        task.prompts.forEach(function (p) { p.y += shift; });
      });
    });

    var last = laid[laid.length - 1];
    return {
      width: last.x + last.reach + MARGIN,
      height: shift + below + MARGIN,
      spineY: shift,
      spine: { x1: MARGIN / 2, x2: last.x + last.reach + MARGIN / 2, y: shift },
      days: laid,
      links: links(graph, laid, shift, above),
      hard: HARD
    };
  }

  // links join days that came back to the same files. They bow away from the
  // spine so they never read as part of the structure.
  function links(graph, laid, shift, above) {
    var at = {};
    laid.forEach(function (day) { at[day.id] = day; });

    // Well clear of the highest node, so a link never crosses the work.
    var lift = shift - above - MARGIN * 0.35;

    return (graph.links || []).map(function (link) {
      var from = at[link.from];
      var to = at[link.to];
      if (!from || !to) return null;
      return {
        from: link.from,
        to: link.to,
        weight: link.weight,
        files: link.files,
        path: "M" + from.x + "," + from.y +
          " C" + from.x + "," + lift + " " + to.x + "," + lift + " " + to.x + "," + to.y
      };
    }).filter(Boolean);
  }

  root.BoughLayout = {
    build: build,
    HARD: HARD,
    READABLE: READABLE,
    COMFORTABLE: COMFORTABLE,
    wholeScale: wholeScale,
    homeScale: homeScale
  };
})(typeof module !== "undefined" && module.exports ? module.exports : window);
