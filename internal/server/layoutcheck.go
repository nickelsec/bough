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

for (const file of files) {
  const graph = JSON.parse(fs.readFileSync(file, "utf8"));
  const label = path.basename(file, ".json");

  for (const mode of ["overview", "reading"]) {
    const out = L.build(graph, mode);
    const tag = label + "/" + mode;

    // Limbs sitting on top of each other is the failure that makes a tree
    // unreadable, and it is what a pure time axis produces.
    for (let i = 1; i < out.limbs.length; i++) {
      const gap = out.limbs[i].y - out.limbs[i - 1].y;
      check(tag + " limbs apart", gap >= 20, "gap " + Math.round(gap) + "px");
    }

    // Nothing may be drawn outside the canvas it is measured against.
    let deepest = 0;
    let leftmost = Infinity;
    let rightmost = 0;
    for (const limb of out.limbs) {
      for (const branch of limb.branches) {
        for (const leaf of branch.leaves) {
          deepest = Math.max(deepest, leaf.y + leaf.r);
          leftmost = Math.min(leftmost, leaf.x - leaf.r);
          rightmost = Math.max(rightmost, leaf.x + leaf.r);
        }
      }
    }
    check(tag + " fits vertically", deepest <= out.height,
      "deepest " + Math.round(deepest) + " vs " + Math.round(out.height));
    check(tag + " fits horizontally", rightmost <= out.width,
      "rightmost " + Math.round(rightmost) + " vs " + Math.round(out.width));
    check(tag + " nothing off the left", leftmost === Infinity || leftmost > 0,
      "leftmost " + Math.round(leftmost));

    // Thickness carries meaning, so it has to stay inside the range that
    // reads. Outside it the shape either disappears or turns to slabs.
    for (const limb of out.limbs) {
      check(tag + " limb weight", limb.width >= 6 && limb.width <= 22,
        limb.width.toFixed(1) + "px");
      for (const branch of limb.branches) {
        check(tag + " branch weight", branch.width >= 2 && branch.width <= 9,
          branch.width.toFixed(1) + "px");
      }
    }

    // The same history must always draw the same way, or a screenshot taken
    // twice shows two different trees.
    check(tag + " deterministic",
      JSON.stringify(out) === JSON.stringify(L.build(graph, mode)));
  }

  const overview = L.build(graph, "overview");
  const tasks = overview.limbs.reduce((n, l) => n + l.branches.length, 0);
  console.log(
    "  " + label.padEnd(8) +
    String(overview.limbs.length).padStart(3) + " limbs " +
    String(tasks).padStart(4) + " branches  " +
    Math.round(overview.width) + "x" + Math.round(overview.height)
  );
}

if (failures) {
  console.log(failures + " failures");
  process.exit(1);
}
console.log("layout holds up");
`
