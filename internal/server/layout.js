// Works out where the tree goes.
//
// Nothing here draws anything. It takes the graph and returns coordinates,
// thicknesses and path strings, which keeps it testable: a tangled tree or a
// runaway canvas fails an assertion instead of being noticed by eye later.
//
// Every number is derived from the data or from a constant below, and nothing
// is random, so the same history always produces the same drawing and a
// screenshot of it is stable.
(function (root) {
  "use strict";

  var TRUNK_X = 520;

  // Two forms. The overview packs everything in so the whole shape fits a
  // window; the reading form gives each limb the room its label needs. The
  // difference matters: fitting the reading form into a laptop window scales it
  // to about a fifth, at which the thickest limb is four pixels and the shape
  // stops being a shape.
  var FORM = {
    overview: { head: 40, perTask: 30, gapMin: 30, gapMax: 110, reach: 300 },
    reading: { head: 70, perTask: 52, gapMin: 44, gapMax: 200, reach: 420 }
  };

  var LIMB = { min: 6, max: 22 };
  var BRANCH = { min: 2, max: 9 };
  var TOP = 90;

  // A branch at or above this counts as hard, and is drawn shorter, thicker
  // and more crooked. The score behind it is a guess, so it only ever changes
  // the texture and never appears as a figure.
  var HARD = 0.5;

  // hash turns an id into a number, so a branch always grows the same way
  // rather than wandering between runs.
  function hash(text) {
    var h = 2166136261;
    for (var i = 0; i < text.length; i++) {
      h ^= text.charCodeAt(i);
      h = (h * 16777619) >>> 0;
    }
    return h;
  }

  // jitter gives a repeatable value between -1 and 1 for a given id and salt.
  function jitter(id, salt) {
    return ((hash(id + ":" + salt) % 2000) / 1000) - 1;
  }

  function clamp(v, lo, hi) {
    return v < lo ? lo : v > hi ? hi : v;
  }

  // scale maps a value into a thickness. The square root matters: with linear
  // scaling one busy day flattens everything else to a hairline.
  function scale(value, most, range) {
    if (!most) return range.min;
    return range.min + (range.max - range.min) * Math.sqrt(clamp(value / most, 0, 1));
  }

  function days(a, b) {
    return (new Date(b) - new Date(a)) / 86400000;
  }

  // build returns everything needed to draw one tree.
  function build(graph, mode) {
    var form = FORM[mode] || FORM.overview;
    var goals = graph.goals || [];
    if (!goals.length) {
      return { width: 0, height: 0, trunk: "", limbs: [], vines: [] };
    }

    var mostGoalEdits = 0;
    var mostTaskEdits = 0;
    goals.forEach(function (goal) {
      mostGoalEdits = Math.max(mostGoalEdits, goal.stats.edits || 0);
      goal.tasks.forEach(function (task) {
        mostTaskEdits = Math.max(mostTaskEdits, task.stats.edits || 0);
      });
    });

    var limbs = [];
    var y = TOP;
    var previousEnd = null;
    var widest = TRUNK_X;

    goals.forEach(function (goal, index) {
      // A pure time axis collides: two sittings on the same day land close
      // enough to overlap. Time decides the pause, content decides the room.
      if (previousEnd) {
        var apart = Math.max(0, days(previousEnd, goal.stats.start));
        y += Math.min(form.gapMax, form.gapMin + (form.gapMax - form.gapMin) * Math.sqrt(clamp(apart / 3, 0, 1)));
      }

      var limb = {
        id: goal.id,
        goal: goal,
        y: y,
        width: scale(goal.stats.edits || 0, mostGoalEdits, LIMB),
        branches: []
      };

      goal.tasks.forEach(function (task, i) {
        // Alternating sides keeps a busy sitting from growing into a fan on
        // one side of the trunk.
        var side = i % 2 === 0 ? 1 : -1;
        var row = Math.floor(i / 2);
        var hard = (task.stats.struggle || 0) >= HARD;

        // Hard work grows shorter and crooked; easy work reaches out clean.
        var reach = form.reach * (hard ? 0.62 : 1) * (0.78 + 0.22 * Math.abs(jitter(task.id, "reach")));
        var rise = row * form.perTask + form.perTask * 0.5;
        var droop = jitter(task.id, "droop") * form.perTask * 0.3;

        var x0 = TRUNK_X;
        var y0 = y + rise * 0.35;
        var x1 = TRUNK_X + side * reach;
        var y1 = y + rise + droop;

        // Control points bow the branch away from the trunk, and a hard one
        // kinks partway along instead of running smooth.
        var bend = hard ? 0.34 + Math.abs(jitter(task.id, "bend")) * 0.3 : 0.55;
        var cx1 = TRUNK_X + side * reach * bend * 0.45;
        var cy1 = y0 + (y1 - y0) * (hard ? 0.75 : 0.35);
        var cx2 = TRUNK_X + side * reach * bend;
        var cy2 = y1 - (hard ? (y1 - y0) * 0.2 : 0);

        limb.branches.push({
          id: task.id,
          task: task,
          hard: hard,
          side: side,
          width: scale(task.stats.edits || 0, mostTaskEdits, BRANCH),
          tip: { x: x1, y: y1 },
          path: "M" + x0 + "," + y0 +
            " C" + cx1 + "," + cy1 + " " + cx2 + "," + cy2 + " " + x1 + "," + y1,
          leaves: leaves(task, x1, y1, side, hard)
        });

        widest = Math.max(widest, Math.abs(x1 - TRUNK_X) + TRUNK_X + 40);
      });

      limbs.push(limb);
      y += form.head + form.perTask * Math.ceil(goal.tasks.length / 2);
      previousEnd = goal.stats.end;
    });

    var bottom = y + 60;
    return {
      width: widest + 60,
      height: bottom,
      trunkX: TRUNK_X,
      trunk: trunk(bottom),
      limbs: limbs,
      vines: vines(graph, limbs),
      hard: HARD
    };
  }

  // leaves places a mark for each prompt along the end of a branch. On a hard
  // branch they bunch up, which is what a bough does when growth was difficult.
  function leaves(task, x, y, side, hard) {
    var out = [];
    var count = Math.min(task.turns.length, 14);
    var spread = hard ? 11 : 20;

    for (var i = 0; i < count; i++) {
      var along = count === 1 ? 0 : (i / (count - 1)) - 0.5;
      out.push({
        x: x + side * along * spread * 1.4 + jitter(task.id + i, "lx") * 3,
        y: y + along * spread + jitter(task.id + i, "ly") * 3,
        r: 2.2 + Math.abs(jitter(task.id + i, "lr")) * 0.9
      });
    }
    return out;
  }

  // trunk draws the spine, with a slight taper so it reads as grown rather
  // than ruled.
  function trunk(bottom) {
    return "M" + TRUNK_X + "," + (TOP - 40) + " L" + TRUNK_X + "," + bottom;
  }

  // vines join sittings that returned to the same files. They arc well clear
  // of the trunk so they never read as part of the hierarchy.
  function vines(graph, limbs) {
    var at = {};
    limbs.forEach(function (limb) { at[limb.id] = limb; });

    return (graph.links || []).map(function (link) {
      var from = at[link.from];
      var to = at[link.to];
      if (!from || !to) return null;

      var swing = TRUNK_X - 120 - Math.min(90, link.weight * 2);
      return {
        from: link.from,
        to: link.to,
        weight: link.weight,
        files: link.files,
        path: "M" + TRUNK_X + "," + from.y +
          " C" + swing + "," + from.y + " " + swing + "," + to.y + " " + TRUNK_X + "," + to.y
      };
    }).filter(Boolean);
  }

  root.BoughLayout = { build: build, HARD: HARD, TRUNK_X: TRUNK_X };
})(typeof module !== "undefined" && module.exports ? module.exports : window);
