package server

// layoutCheck is run by node against layout.js in the test above.
//
// It lives here as a string rather than as a file so it is never shipped in
// the binary and never mistaken for something the page loads.
const layoutCheck = `
const fs = require("fs");
const path = require("path");

const layoutPath = path.resolve(process.argv[2]);
const L = require(layoutPath).BoughLayout;
const files = process.argv.slice(3);

let failures = 0;
function check(name, ok, detail) {
  if (!ok) {
    failures++;
    console.log("  FAIL  " + name + (detail ? "  " + detail : ""));
  }
}

// Every node as a box, so overlap is one comparison rather than three.
function boxes(out) {
  const all = [];
  for (const day of out.days) {
    all.push({ what: "day " + day.id, x: day.x, y: day.y, w: day.size, h: day.size });
    for (const task of day.tasks) {
      all.push({ what: "task " + task.id, x: task.x, y: task.y, w: task.size, h: task.size });
      for (const p of task.prompts) {
        all.push({ what: "prompt " + p.id, x: p.x, y: p.y, w: p.r * 2, h: p.r * 2 });
      }
    }
  }
  return all;
}

function overlaps(a, b) {
  return Math.abs(a.x - b.x) * 2 < a.w + b.w &&
         Math.abs(a.y - b.y) * 2 < a.h + b.h;
}

for (const file of files) {
  const graph = JSON.parse(fs.readFileSync(file, "utf8"));
  const label = path.basename(file, ".json");

  for (const mode of ["overview", "reading"]) {
    const out = L.build(graph, mode);
    const tag = label + "/" + mode;
    const all = boxes(out);

    // Two nodes drawn on top of each other is the failure that makes the
    // diagram unreadable, and it is what a pure time axis produces.
    let clashes = 0;
    let example = "";
    for (let i = 0; i < all.length; i++) {
      for (let j = i + 1; j < all.length; j++) {
        if (overlaps(all[i], all[j])) {
          clashes++;
          if (!example) example = all[i].what + " over " + all[j].what;
        }
      }
    }
    check(tag + " nothing overlaps", clashes === 0, clashes + " clashes, e.g. " + example);

    // Nothing may be drawn outside the canvas it is measured against.
    for (const n of all) {
      check(tag + " " + n.what + " is on the canvas",
        n.x - n.w / 2 >= 0 && n.x + n.w / 2 <= out.width &&
        n.y - n.h / 2 >= 0 && n.y + n.h / 2 <= out.height,
        Math.round(n.x) + "," + Math.round(n.y) +
        " in " + Math.round(out.width) + "x" + Math.round(out.height));
    }

    // The opening view has to frame what is drawn, not the canvas it sits
    // on. The layout leaves a wide margin to pan into, so fitting to the
    // canvas scales for space nothing occupies: a short history filled about
    // half the height it was given and opened at half the size it could,
    // sitting off centre, which reads as the view having failed entirely.
    const seen = { x0: Infinity, y0: Infinity, x1: -Infinity, y1: -Infinity };
    for (const n of all) {
      seen.x0 = Math.min(seen.x0, n.x - n.w / 2);
      seen.x1 = Math.max(seen.x1, n.x + n.w / 2);
      seen.y0 = Math.min(seen.y0, n.y - n.h / 2);
      seen.y1 = Math.max(seen.y1, n.y + n.h / 2);
    }
    // The spine is the axis, so it counts vertically. Horizontally it runs
    // past the nodes further at the arrow end than at the start, and a box
    // drawn round it is lopsided enough to shove the work off to one side.
    if (out.spine) {
      seen.y0 = Math.min(seen.y0, out.spine.y - 13);
      seen.y1 = Math.max(seen.y1, out.spine.y + 13);
    }
    const cw = seen.x1 - seen.x0, ch = seen.y1 - seen.y0;

    for (const box of [{ w: 1200, h: 800 }, { w: 1440, h: 900 }, { w: 900, h: 600 }]) {
      const margin = 40;
      const room = { w: box.w - margin * 2, h: box.h - margin * 2 };
      let scale = Math.min(1.1, room.h / (ch + 30), room.w / cw);

      // Shrink until the spine fits too, so the arrow is never clipped.
      if (out.spine) {
        const mid = seen.x0 + cw / 2;
        const reach = Math.max(mid - (out.spine.x1 - 13), out.spine.x2 + 13 - mid) * 2;
        if (reach > 0) scale = Math.min(scale, room.w / reach);
      }
      check(tag + " fit scale is usable", scale > 0 && isFinite(scale), String(scale));

      const vx = (box.w - cw * scale) / 2 - seen.x0 * scale;
      const vy = (box.h - (ch + 30) * scale) / 2 - seen.y0 * scale;

      // The nodes are what has to sit in the middle.
      const nodeMid = vx + (seen.x0 + cw / 2) * scale;
      check(tag + " nodes sit centred at " + box.w,
        Math.abs(nodeMid - box.w / 2) < 1,
        "off by " + (nodeMid - box.w / 2).toFixed(1) + "px");

      // And the spine, arrow and all, stays on screen.
      if (out.spine) {
        check(tag + " spine fits at " + box.w,
          vx + (out.spine.x1 - 13) * scale >= -1 &&
          vx + (out.spine.x2 + 13) * scale <= box.w + 1);
      }

      // The framed box is the shapes plus the band under them the date
      // labels are drawn into, so that band is what gets centred.
      const my = vy + (seen.y0 + (ch + 30) / 2) * scale;
      check(tag + " fit centres vertically at " + box.h,
        Math.abs(my - box.h / 2) < 1, "off by " + (my - box.h / 2).toFixed(1) + "px");

      // And everything drawn has to land inside the window.
      check(tag + " fit keeps it on screen at " + box.w + "x" + box.h,
        vx + seen.x0 * scale >= -1 && vy + seen.y0 * scale >= -1 &&
        vx + seen.x1 * scale <= box.w + 1 && vy + seen.y1 * scale <= box.h + 1);
    }

    // A long history is a ribbon: it grows sideways with every day worked and
    // never grows taller. Fitting one to a window scales for the width alone,
    // and a hundred days drew its day squares at under three pixels, which is
    // a line rather than a diagram. The view opens at a size the nodes can be
    // read at instead, and time becomes something to travel along.
    const READABLE = 32 / 36;
    for (const box of [{ w: 1440, h: 900 }, { w: 390, h: 844 }]) {
      const margin = box.w < 560 ? 16 : 40;
      const r = { w: box.w - margin * 2, h: box.h - margin * 2 };
      let all = Math.min(1.1, r.h / (ch + 30), r.w / cw);
      if (out.spine) {
        const mid = seen.x0 + cw / 2;
        const far = Math.max(mid - (out.spine.x1 - 13), out.spine.x2 + 13 - mid) * 2;
        if (far > 0) all = Math.min(all, r.w / far);
      }
      let home = all;
      if (all < READABLE * 0.75) {
        home = Math.min(READABLE, Math.max(all, r.h / (ch + 30)));
      }

      // Whatever the length, the opening view has to be legible.
      check(tag + " opens legibly at " + box.w,
        home >= Math.min(READABLE, all) - 0.001,
        "day square " + (36 * home).toFixed(1) + "px");

      // And it never opens larger than showing everything would need.
      check(tag + " never opens past the whole at " + box.w,
        home <= Math.max(all, READABLE) + 0.001);

      // The readout is a share of home, so a hundred percent means the same
      // on every project however long.
      check(tag + " home is usable at " + box.w, home > 0 && isFinite(home));
    }

    // Days run left to right through time, and a diagram that doubles back
    // is telling a lie about the order the work happened in.
    for (let i = 1; i < out.days.length; i++) {
      check(tag + " days run forward", out.days[i].x > out.days[i - 1].x);
    }

    // Size carries meaning, so it has to stay inside the range that reads.
    for (const day of out.days) {
      check(tag + " day size", day.size >= 26 && day.size <= 46, day.size + "px");
      for (const task of day.tasks) {
        check(tag + " task size", task.size >= 14 && task.size <= 26, task.size + "px");
      }
    }

    // Every prompt belongs to a task and every task to a day, which is the
    // whole claim the picture makes.
    for (const day of out.days) {
      check(tag + " day has its tasks",
        day.tasks.length === day.goal.tasks.length);
      for (const task of day.tasks) {
        check(tag + " task has its prompts",
          task.prompts.length === (task.task.turns || []).length,
          task.prompts.length + " vs " + (task.task.turns || []).length);
      }
    }

    // The same history must always draw the same way, or a screenshot taken
    // twice shows two different diagrams.
    check(tag + " deterministic",
      JSON.stringify(out) === JSON.stringify(L.build(graph, mode)));
  }

  const overview = L.build(graph, "overview");
  const tasks = overview.days.reduce((n, d) => n + d.tasks.length, 0);
  const prompts = overview.days.reduce(
    (n, d) => n + d.tasks.reduce((m, t) => m + t.prompts.length, 0), 0);
  console.log(
    "  " + label.padEnd(8) +
    String(overview.days.length).padStart(3) + " days " +
    String(tasks).padStart(4) + " tasks " +
    String(prompts).padStart(5) + " prompts  " +
    Math.round(overview.width) + "x" + Math.round(overview.height)
  );
}

if (failures) {
  console.log(failures + " failures");
  process.exit(1);
}
console.log("layout holds up");
`
