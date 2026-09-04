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

  var MARGIN = 120;
  var SPINE_Y = 0; // filled in once the tallest column is known

  // A task at or above this counts as hard. The score behind it is a guess, so
  // it only ever changes the colour and never appears as a figure.
  var HARD = 0.5;

  function clamp(v, lo, hi) {
    return v < lo ? lo : v > hi ? hi : v;
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
    goals.forEach(function (goal) {
      mostDayEdits = Math.max(mostDayEdits, goal.stats.edits || 0);
      goal.tasks.forEach(function (task) {
        mostTaskEdits = Math.max(mostTaskEdits, task.stats.edits || 0);
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

  root.BoughLayout = { build: build, HARD: HARD };
})(typeof module !== "undefined" && module.exports ? module.exports : window);
