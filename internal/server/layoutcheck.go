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
