package server

// spendCheck is run by node against bough.js in spend_test.go.
//
// It lifts the handful of functions the cost filter and the money column are
// built on out of the page and exercises them directly. They are small, pure
// and entirely arithmetic, which is exactly the sort of thing that is cheap to
// check here and expensive to notice on screen: a figure that is wrong by a
// factor still looks like a figure.
const spendCheck = `
const fs = require("fs");
const src = fs.readFileSync(process.argv[2], "utf8");

// Pull one named function out of the page source, with whatever it needs.
// The page is one long closure over a graph, so it cannot be required; the
// functions under test are self-contained and can be lifted out whole.
function lift(name) {
  const at = src.indexOf("function " + name + "(");
  if (at < 0) throw new Error("no function " + name + " in bough.js");
  // Walk braces from the first one after the signature to find the body's end.
  let i = src.indexOf("{", at), depth = 0, end = -1;
  for (; i < src.length; i++) {
    if (src[i] === "{") depth++;
    else if (src[i] === "}") { depth--; if (depth === 0) { end = i + 1; break; } }
  }
  if (end < 0) throw new Error("unbalanced body for " + name);
  return src.slice(at, end);
}

// The graph the lifted code closes over, and the filter state it reads.
let graph = { goals: [], totals: {} };
let filters = { least: 0, by: "cost" };

eval(lift("money"));
eval(lift("tokensOf"));
eval(lift("figureOf"));
eval(lift("dear"));
eval(lift("odd"));
eval(lift("big"));
eval(lift("modelShare"));

let failures = 0;
function is(got, want, what) {
  const a = JSON.stringify(got), b = JSON.stringify(want);
  if (a !== b) { console.log("FAIL " + what + ": got " + a + ", want " + b); failures++; }
}

// --- money -----------------------------------------------------------------
// Whole dollars once past ten, cents below it, and enough places under a cent
// that a real figure never reads as free.
is(money(1074.357), "$1,074", "money rounds a large figure and groups it");
is(money(85.24), "$85", "money rounds past ten");
is(money(3.74), "$3.74", "money keeps cents under ten");
is(money(0.5755), "$0.58", "money keeps cents just under a dollar");
is(money(0.0004), "$0.0004", "money keeps places under a cent");
is(money(0), "$0", "nothing costs nothing");

// --- figureOf ---------------------------------------------------------------
// Null means no figure, which is not the same as zero. Everything downstream
// turns on that distinction.
const priced = { cost: 12.5, tokens: { input: 1, output: 2, cacheRead: 3, cacheWrite: 4 } };
is(figureOf(priced, "cost"), 12.5, "cost is read");
is(figureOf(priced, "total"), 10, "total sums the four");
is(figureOf(priced, "output"), 2, "a single count is read");
is(figureOf({ tokens: priced.tokens }, "cost"), null, "no cost is null, not zero");
is(figureOf({ cost: 1 }, "total"), null, "no tokens is null");
is(figureOf(null, "cost"), null, "no stats at all is null");
is(figureOf({ cost: 0, tokens: {} }, "cost"), 0, "a real zero cost is zero");

// --- dear ------------------------------------------------------------------
// Work with no figure must never satisfy a filter. It has not been shown to
// cost less than the bar; it has not been priced at all, and lighting it
// beside work that was priced would say the wrong thing about both.
filters = { least: 10, by: "cost" };
is(dear({ cost: 25 }), true, "dearer work stays lit");
is(dear({ cost: 10 }), true, "work exactly at the bar stays lit");
is(dear({ cost: 9.99 }), false, "cheaper work fades");
is(dear({ tokens: { output: 5 } }), false, "unpriced work never clears a cost bar");
is(dear({}), false, "work with no figures never clears the bar");
is(dear(null), false, "nothing never clears the bar");

filters = { least: 0, by: "cost" };
is(dear({ cost: 0 }), true, "a zero bar admits a zero figure");

// --- odd -------------------------------------------------------------------
// The model is named only when a project used more than one. On a project
// with a single model it would be one constant printed on every row.
graph = { goals: [], totals: { models: { "claude-opus-5": {} } } };
is(odd({ models: { "claude-opus-5": {} } }), "", "one model project names none");

graph = { goals: [], totals: { models: { "claude-opus-5": {}, "gpt-6-astra": {} } } };
is(odd({ models: { "gpt-6-astra": {} } }), "gpt-6-astra", "a mixed project names the model");
is(odd({ models: { "gpt-6-astra": {}, "claude-opus-5": {} } }),
   "claude-opus-5, gpt-6-astra", "both are named, sorted");
is(odd({}), "", "work with no models names none");
is(odd(null), "", "nothing names none");

// --- modelShare ------------------------------------------------------------
// The share is of what each model wrote, since cache reads swamp everything
// and say how long the conversation was rather than who did the work.
is(modelShare({ a: { output: 300, cacheRead: 1 }, b: { output: 100, cacheRead: 9e9 } }),
   "a 75%  ·  b 25%", "shares go by output, not by total");
is(modelShare({ solo: { output: 5 } }), "solo", "one model is just named");
is(modelShare({ a: { cacheRead: 5 }, b: { cacheRead: 9 } }), "a  ·  b",
   "no output anywhere means no percentages and no divide by zero");
is(modelShare({}), "", "no models says nothing");

if (failures) {
  console.log(failures + " check(s) failed");
  process.exit(1);
}
console.log("spend helpers: every check passed");
`
