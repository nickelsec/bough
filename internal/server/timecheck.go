package server

// timeCheck is run by node against bough.js in time_test.go.
//
// It lifts the helpers that say when a sitting ran and for how long, and checks
// what they say rather than exactly how: the wording of a time comes from the
// viewer's own locale, so "5:21 PM" on one machine is "17:21" on another. What
// has to hold everywhere is the shape: one time when nothing ran on, a range
// when it did, the weekday when it ran past midnight, and nothing at all when
// the record has no time to give.
const timeCheck = `
const fs = require("fs");
const src = fs.readFileSync(process.argv[2], "utf8");

function lift(name) {
  const at = src.indexOf("function " + name + "(");
  if (at < 0) throw new Error("no function " + name + " in bough.js");
  let i = src.indexOf("{", at), depth = 0, end = -1;
  for (; i < src.length; i++) {
    if (src[i] === "{") depth++;
    else if (src[i] === "}") { depth--; if (depth === 0) { end = i + 1; break; } }
  }
  if (end < 0) throw new Error("unbalanced body for " + name);
  return src.slice(at, end);
}

eval(lift("clock"));
eval(lift("span"));
eval(lift("count"));
eval(lift("duration"));
eval(lift("figures"));

let failures = 0;
function ok(cond, what) {
  if (!cond) { console.log("FAIL " + what); failures++; }
}

// --- clock -----------------------------------------------------------------
const start = "2026-09-25T17:21:13+05:30";
ok(clock(start) !== "" && /\d/.test(clock(start)), "a time gives a time");
ok(clock("") === "" && clock(undefined) === "", "no time gives nothing");
ok(clock("not a time") === "", "an unreadable time gives nothing");

// --- span ------------------------------------------------------------------
const same = { start: start, end: "2026-09-25T17:21:40+05:30" };
ok(span(same) === clock(start), "the same minute gives the start alone");

const ran = { start: start, end: "2026-09-25T17:28:57+05:30" };
ok(span(ran) === clock(ran.start) + " to " + clock(ran.end), "a range reads start to end");

// Past midnight in the viewer's own zone, whatever that is: build both ends
// from local dates so the check does not depend on the machine it runs on.
const late = new Date(2026, 8, 25, 23, 40), early = new Date(2026, 8, 26, 1, 10);
const overnight = { start: late.toISOString(), end: early.toISOString() };
const weekday = early.toLocaleDateString([], { weekday: "short" });
ok(span(overnight).indexOf(" to " + weekday + " ") > 0, "past midnight names the day it ended");

ok(span({}) === "" && span(null) === "", "no times give nothing");
ok(span({ start: start }) === clock(start), "a start with no end gives the start");

// --- figures ---------------------------------------------------------------
ok(figures({ turns: 4, edits: 12, activeMinutes: 7 }).indexOf("7 min at the keyboard") > 0,
   "keyboard time is said when there was some");
ok(figures({ turns: 1, activeMinutes: 0 }).indexOf("keyboard") < 0,
   "no keyboard time is left out rather than called a moment");

if (failures) {
  console.log(failures + " check(s) failed");
  process.exit(1);
}
console.log("time helpers: every check passed");
`
