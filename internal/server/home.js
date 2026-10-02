// bough's home page: every project, as a card.
//
// The list arrives inside the page, so the cards draw at once. The figures on
// them take longer, because working them out reads each project's whole
// history, so they are fetched as the server finishes them and filled in where
// a shimmer was holding their place.
(function () {
  "use strict";

  var home = window.BOUGH_HOME;
  if (!home) return;

  var cards = home.cards || [];
  var token = home.token;
  var sums = {};
  var failed = {};

  var list = document.getElementById("cards");
  var q = document.getElementById("q");
  var sortBtn = document.getElementById("sort");
  var sortMenu = document.getElementById("sort-menu");
  var sortLabel = document.getElementById("sort-label");
  var sortBy = { value: "recent" };
  var agentsBox = document.getElementById("agents");
  var status = document.getElementById("status");
  var count = document.getElementById("home-count");
  var nothing = document.getElementById("nothing");
  var nothingQ = document.getElementById("nothing-q");

  var agent = "";
  var editing = null;

  // A choice of sort is remembered, since somebody who sorts by cost once
  // probably wants it next time too. Storage can be refused, and then the
  // page simply forgets.
  try {
    var kept = window.localStorage.getItem("bough.sort");
    if (kept && sortMenu.querySelector('[data-value="' + kept + '"]')) sortBy.value = kept;
  } catch (e) { /* forgetting is fine */ }

  // ---- building the page -------------------------------------------------

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  function pencil() {
    var b = el("button", "rename");
    b.type = "button";
    b.title = "Rename";
    b.setAttribute("aria-label", "Rename this project");
    b.innerHTML = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M10.5 2.5l3 3L6 13H3v-3z"/><path d="M9 4l3 3"/></svg>';
    return b;
  }

  function cardFor(c) {
    var li = el("li", "card");
    li.dataset.id = c.id;

    var top = el("div", "card-top");
    top.appendChild(el("span", "badge", c.agentName || c.agent));
    if (c.here) top.appendChild(el("span", "badge is-here", "Here"));
    var whenText = ago(c.lastWorked);
    if (whenText) {
      var w = el("span", "card-when", whenText);
      w.title = "Last worked " + new Date(c.lastWorked).toLocaleString();
      top.appendChild(w);
    }
    li.appendChild(top);

    var name = el("div", "card-name");
    var h = el("h2");
    var a = el("a", "card-open", c.name);
    a.href = "/p/" + encodeURIComponent(c.id);
    // The name is kept to one line and cut short if it has to be, so the
    // whole of it is there on hover.
    a.title = c.name;
    h.appendChild(a);
    name.appendChild(h);
    var edit = pencil();
    name.appendChild(edit);
    li.appendChild(name);

    var path = el("p", "card-path");
    // Reversed so the end of a long path, the part that names the project,
    // is what stays when it is cut short. The bidi mark keeps the slashes
    // where they belong.
    path.textContent = "‎" + c.path + "‎";
    path.title = c.folder ? c.path + "\n(folder: " + c.folder + ")" : c.path;
    li.appendChild(path);

    li.appendChild(el("div", "card-body"));
    fillBody(li, c);

    edit.addEventListener("click", function (ev) {
      ev.preventDefault();
      startRename(li, c);
    });
    a.addEventListener("click", function (ev) {
      if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.button !== 0) return;
      li.classList.add("opening");
    });
    return li;
  }

  // fillBody draws the figures, or holds their place until they arrive.
  function fillBody(li, c) {
    var body = li.querySelector(".card-body");
    body.textContent = "";
    var s = sums[c.id];

    if (failed[c.id]) {
      body.appendChild(el("p", "card-failed", "This history could not be read."));
      return;
    }

    var dl = el("dl", "card-figures");
    figure(dl, "Keyboard", s ? duration(s.activeMinutes, s.prompts) : null);
    figure(dl, "Cost", s ? costText(s) : null, s && s.local ? "local" : "");
    figure(dl, "Prompts", s ? s.prompts.toLocaleString() : null);
    body.appendChild(dl);

    // Always two lines, so every card is the same shape: how much work there
    // was, then when it ran and what did it. One model is named, and the rest
    // counted, with all of them on hover.
    var foot = el("div", "card-foot");
    var counts = el("div", "foot-line");
    var context = el("div", "foot-line");
    if (s) {
      counts.appendChild(el("span", "", plural(s.sittings, "sitting")));
      counts.appendChild(el("span", "", plural(s.tasks, "task")));
      counts.appendChild(el("span", "", plural(s.commits, "commit")));
      context.appendChild(el("span", "card-dates", period(s.start, s.end)));
      if (s.models && s.models.length) {
        var m = el("span", "card-model");
        m.appendChild(el("span", "card-model-name", s.models[0]));
        if (s.models.length > 1) m.appendChild(el("span", "card-model-more", "+" + (s.models.length - 1)));
        m.title = s.models.join("\n");
        context.appendChild(m);
      }
    } else {
      counts.appendChild(el("span", "wait"));
      context.appendChild(el("span", "wait wide"));
    }
    foot.appendChild(counts);
    foot.appendChild(context);
    body.appendChild(foot);
  }

  function figure(dl, label, value, note) {
    var d = el("div", "figure");
    d.appendChild(el("dt", "", label));
    var dd = el("dd");
    if (value == null) {
      dd.appendChild(el("span", "wait"));
    } else {
      dd.textContent = value;
      if (note) {
        dd.appendChild(document.createTextNode(" "));
        dd.appendChild(el("small", "", note));
      }
    }
    d.appendChild(dd);
    dl.appendChild(d);
  }

  // ---- what to show, and in what order ----------------------------------

  function matches(c) {
    if (agent && c.agent !== agent) return false;
    var text = q.value.trim().toLowerCase();
    if (!text) return true;
    var hay = [c.name, c.folder, c.path, c.agentName].join(" ").toLowerCase();
    return text.split(/\s+/).every(function (w) { return hay.indexOf(w) >= 0; });
  }

  function ordered() {
    var by = sortBy.value;
    var shown = cards.filter(matches);
    var key = {
      time: function (c) { return sums[c.id] ? sums[c.id].activeMinutes : -1; },
      cost: function (c) { return sums[c.id] && sums[c.id].cost != null ? sums[c.id].cost : -1; }
    }[by];
    if (by === "name") {
      shown.sort(function (a, b) { return a.name.localeCompare(b.name); });
    } else if (key) {
      shown.sort(function (a, b) { return key(b) - key(a); });
    }
    // "Most recent" is the order the server sent, with the project bough was
    // run from first.
    return shown;
  }

  function draw() {
    if (editing) return;
    var shown = ordered();
    list.textContent = "";
    shown.forEach(function (c) { list.appendChild(cardFor(c)); });

    var filtering = q.value.trim() || agent;
    nothing.hidden = shown.length > 0 || !filtering;
    nothingQ.textContent = q.value.trim() ? "“" + q.value.trim() + "”" : "that filter";

    count.textContent = filtering
      ? shown.length + " of " + plural(cards.length, "project")
      : plural(cards.length, "project") + " with history on this machine";
  }

  function agentFilter() {
    var seen = {};
    cards.forEach(function (c) { seen[c.agent] = c.agentName || c.agent; });
    var keys = Object.keys(seen);
    if (keys.length < 2) return;
    agentsBox.hidden = false;
    var choices = [["", "All"]].concat(keys.map(function (k) { return [k, seen[k]]; }));
    choices.forEach(function (ch) {
      var b = el("button", "", ch[1]);
      b.type = "button";
      b.setAttribute("aria-pressed", ch[0] === agent ? "true" : "false");
      b.addEventListener("click", function () {
        agent = ch[0];
        agentsBox.querySelectorAll("button").forEach(function (x) {
          x.setAttribute("aria-pressed", x === b ? "true" : "false");
        });
        draw();
      });
      agentsBox.appendChild(b);
    });
  }

  // ---- renaming ----------------------------------------------------------

  function startRename(li, c) {
    if (editing) return;
    editing = c.id;
    var box = li.querySelector(".card-name");
    var before = box.innerHTML;
    box.textContent = "";

    var field = nameField(c.name, c.folder, finish);
    var input = field.input;
    box.appendChild(field.wrap);
    input.focus();
    input.select();

    var done = false;
    function finish(save) {
      if (done) return;
      done = true;
      var wanted = input.value.trim();
      if (!save || wanted === c.name) {
        box.innerHTML = before;
        rebind(li, c);
        editing = null;
        return;
      }
      input.disabled = true;
      rename(c.id, wanted).then(function (name) {
        // Every card for the same folder takes the name, since a name
        // belongs to the folder rather than to one agent's history.
        cards.forEach(function (x) {
          if (x.path === c.path) {
            if (!x.folder && name !== x.name) x.folder = x.name;
            x.name = name;
          }
        });
        editing = null;
        draw();
        say("Renamed to " + name, "done");
      }, function (err) {
        box.innerHTML = before;
        rebind(li, c);
        editing = null;
        say("Could not rename: " + err.message, "error");
      });
    }
  }

  // nameField is the box a name is edited in: the name, with a tick to keep
  // it and a cross to leave it as it was, inside the box's right edge. Enter
  // and Esc do the same. Clicking away keeps what was typed.
  //
  // Emptied, the box shows the folder's name as its placeholder, since that
  // is the name the project goes back to.
  function nameField(value, folder, finish) {
    var wrap = el("div", "name-field");
    var input = el("input", "name-edit");
    input.value = value;
    input.maxLength = 80;
    input.placeholder = folder || "";
    input.setAttribute("aria-label", "New name for this project");
    wrap.appendChild(input);

    var ok = el("button", "name-act name-ok");
    ok.type = "button";
    ok.title = "Save (Enter)";
    ok.setAttribute("aria-label", "Save the name");
    ok.innerHTML = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3.5 8.5l3 3 6-7"/></svg>';
    var no = el("button", "name-act name-no");
    no.type = "button";
    no.title = "Cancel (Esc)";
    no.setAttribute("aria-label", "Cancel");
    no.innerHTML = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M4.5 4.5l7 7M11.5 4.5l-7 7"/></svg>';
    wrap.appendChild(ok);
    wrap.appendChild(no);

    // Pressing a button would take the focus from the box first, and leaving
    // the box saves. Holding the focus lets the cross mean cancel.
    [ok, no].forEach(function (b) {
      b.addEventListener("pointerdown", function (ev) { ev.preventDefault(); });
    });
    ok.addEventListener("click", function (ev) { ev.preventDefault(); finish(true); });
    no.addEventListener("click", function (ev) { ev.preventDefault(); finish(false); });
    input.addEventListener("keydown", function (ev) {
      if (ev.key === "Enter") { ev.preventDefault(); finish(true); }
      if (ev.key === "Escape") { ev.preventDefault(); finish(false); }
    });
    input.addEventListener("blur", function () { finish(true); });
    return { wrap: wrap, input: input };
  }

  // rebind puts the pencil's handler back after the name was redrawn from
  // markup, which drops listeners.
  function rebind(li, c) {
    var b = li.querySelector(".rename");
    if (b) b.addEventListener("click", function (ev) { ev.preventDefault(); startRename(li, c); });
    var a = li.querySelector(".card-open");
    if (a) a.addEventListener("click", function (ev) {
      if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.button !== 0) return;
      li.classList.add("opening");
    });
  }

  function rename(id, name) {
    return fetch("/api/name", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Bough-Token": token },
      body: JSON.stringify({ id: id, name: name })
    }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t.trim() || r.statusText); });
      return r.json();
    }).then(function (j) { return j.name; });
  }

  // ---- the figures, as they come in --------------------------------------

  // The line under the toolbar says one thing at a time, as a small chip: the
  // history still being read, or what just happened. Something that just
  // happened holds the line for a few seconds before the reading count, if
  // there still is one, comes back.
  var quiet = null;
  var holding = false;

  // chip shows a message. The same kind of message already showing only has
  // its words changed: the reading count is asked for every half second, and
  // building the chip anew each time replayed its entrance and restarted the
  // ring, so it twitched.
  function chip(text, kind) {
    if (!text) {
      status.textContent = "";
      return;
    }
    var c = status.firstChild;
    if (c && c.classList.contains("is-" + kind)) {
      var words = c.lastChild;
      if (words.textContent !== text) words.textContent = text;
      return;
    }
    status.textContent = "";
    c = el("span", "status-chip is-" + kind);
    c.appendChild(el("span", "status-icon"));
    c.appendChild(el("span", "", text));
    status.appendChild(c);
  }

  function say(text, kind) {
    chip(text, kind);
    holding = true;
    clearTimeout(quiet);
    quiet = setTimeout(function () {
      holding = false;
      progress();
    }, 3500);
  }

  function progress() {
    if (holding) return;
    var have = Object.keys(sums).length + Object.keys(failed).length;
    chip(have < cards.length ? "Reading your history " + have + " of " + cards.length : "", "busy");
  }

  function poll() {
    fetch("/api/summaries").then(function (r) { return r.json(); }).then(function (j) {
      var changed = false;
      Object.keys(j.summaries || {}).forEach(function (id) {
        if (!sums[id]) changed = true;
        sums[id] = j.summaries[id];
      });
      Object.keys(j.failed || {}).forEach(function (id) {
        if (!failed[id]) changed = true;
        failed[id] = j.failed[id];
      });
      if (changed) {
        // Only the cards that changed are redrawn, unless the order depends
        // on the figures, in which case the grid is put in order again.
        if (sortBy.value === "time" || sortBy.value === "cost") {
          draw();
        } else {
          cards.forEach(function (c) {
            var li = list.querySelector('[data-id="' + c.id + '"]');
            if (li && editing !== c.id) fillBody(li, c);
          });
        }
      }
      if (!editing) progress();
      if (!j.done) setTimeout(poll, 600);
    }, function () {
      setTimeout(poll, 2000);
    });
  }

  // ---- formatting, the same way the project page does it ----------------

  // duration is time at the keyboard. Under a minute is said as such rather
  // than as nothing, when there was work in it.
  function duration(minutes, prompts) {
    if (!minutes) return prompts ? "<1 min" : "0 min";
    if (minutes < 60) return minutes + " min";
    var hours = Math.floor(minutes / 60);
    var rest = minutes % 60;
    return hours < 10 && rest ? hours + "h " + rest + "m" : hours + "h";
  }

  function money(d) {
    if (d >= 10) return "$" + Math.round(d).toLocaleString("en-US");
    if (d >= 0.01) return "$" + d.toFixed(2);
    if (d > 0) return "$" + d.toFixed(4);
    return "$0";
  }

  // A project with a model nobody publishes a rate for has no figure, which
  // is said as NA rather than shown as nothing or as free.
  function costText(s) {
    return s.cost == null ? "NA" : money(s.cost);
  }

  function plural(n, word) {
    return n.toLocaleString() + " " + word + (n === 1 ? "" : "s");
  }

  function ago(iso) {
    if (!iso) return "";
    var t = new Date(iso);
    if (isNaN(t) || t.getFullYear() < 2000) return "";
    var d = (Date.now() - t.getTime()) / 36e5;
    if (d < 1) return "just now";
    if (d < 24) return Math.floor(d) + "h ago";
    if (d < 48) return "yesterday";
    if (d < 24 * 14) return Math.floor(d / 24) + " days ago";
    if (d < 24 * 60) return Math.floor(d / 24 / 7) + " weeks ago";
    return t.toLocaleDateString([], { month: "short", year: "numeric" });
  }

  function period(a, b) {
    var from = new Date(a), to = new Date(b);
    if (isNaN(from) || from.getFullYear() < 2000) return "";
    var opts = { day: "numeric", month: "short" };
    if (from.getFullYear() !== new Date().getFullYear()) opts.year = "numeric";
    var f = from.toLocaleDateString([], opts);
    var t = isNaN(to) ? f : to.toLocaleDateString([], opts);
    return f === t ? f : f + " to " + t;
  }

  // version names the bough in the footer. A release, "v0.8.0", links to what
  // changed in it. Anything else is a build from a working copy, whose version
  // is a long string of hashes and dates that would mean nothing there, so it
  // says "dev build" and keeps the whole string for hover.
  function version(v) {
    var a = document.getElementById("version");
    if (!v) return;
    a.hidden = false;
    if (/^v\d+\.\d+\.\d+$/.test(v)) {
      a.textContent = v;
      a.href = "https://github.com/nickelsec/bough/releases/tag/" + v;
      a.title = "What changed in " + v;
    } else {
      a.textContent = "dev build";
      a.removeAttribute("href");
      a.title = v;
    }
  }

  // ---- wiring ------------------------------------------------------------

  q.addEventListener("input", draw);
  // ---- the sort menu -----------------------------------------------------

  function options() { return Array.prototype.slice.call(sortMenu.querySelectorAll("li")); }

  function showSort() {
    options().forEach(function (li) {
      var on = li.dataset.value === sortBy.value;
      li.setAttribute("aria-selected", on ? "true" : "false");
      if (on) sortLabel.textContent = li.textContent;
    });
  }

  function mark(li) {
    options().forEach(function (x) { x.classList.toggle("active", x === li); });
  }

  function openSort() {
    sortMenu.hidden = false;
    sortBtn.setAttribute("aria-expanded", "true");
    mark(sortMenu.querySelector('[aria-selected="true"]') || options()[0]);
  }

  function closeSort() {
    sortMenu.hidden = true;
    sortBtn.setAttribute("aria-expanded", "false");
    mark(null);
  }

  function pickSort(value) {
    sortBy.value = value;
    try { window.localStorage.setItem("bough.sort", value); } catch (e) { /* fine */ }
    showSort();
    closeSort();
    draw();
  }

  sortBtn.addEventListener("click", function () {
    if (sortMenu.hidden) openSort(); else closeSort();
  });
  options().forEach(function (li) {
    li.addEventListener("click", function () {
      pickSort(li.dataset.value);
      // Chosen with the pointer, so nothing is left looking selected.
      sortBtn.blur();
    });
    li.addEventListener("pointerenter", function () { mark(li); });
  });
  sortBtn.addEventListener("keydown", function (ev) {
    var all = options();
    var at = all.indexOf(sortMenu.querySelector(".active"));
    if (ev.key === "ArrowDown" || ev.key === "ArrowUp") {
      ev.preventDefault();
      if (sortMenu.hidden) { openSort(); return; }
      var next = ev.key === "ArrowDown" ? Math.min(at + 1, all.length - 1) : Math.max(at - 1, 0);
      mark(all[next]);
    } else if ((ev.key === "Enter" || ev.key === " ") && !sortMenu.hidden && at >= 0) {
      ev.preventDefault();
      pickSort(all[at].dataset.value);
    } else if (ev.key === "Escape" && !sortMenu.hidden) {
      ev.preventDefault();
      closeSort();
    }
  });
  document.addEventListener("pointerdown", function (ev) {
    if (!sortMenu.hidden && !document.getElementById("sort-box").contains(ev.target)) closeSort();
  });
  showSort();
  document.getElementById("clear").addEventListener("click", function () {
    q.value = "";
    agent = "";
    agentsBox.querySelectorAll("button").forEach(function (x, i) {
      x.setAttribute("aria-pressed", i === 0 ? "true" : "false");
    });
    draw();
    q.focus();
  });

  document.addEventListener("keydown", function (ev) {
    var typing = /^(INPUT|SELECT|TEXTAREA)$/.test(document.activeElement.tagName);
    if (ev.key === "/" && !typing) {
      ev.preventDefault();
      q.focus();
      q.select();
    }
    // Enter in the search opens the first card, so three letters and Enter
    // is all it takes to get to a project.
    if (ev.key === "Enter" && document.activeElement === q) {
      var first = list.querySelector(".card-open");
      if (first) first.click();
    }
    if (ev.key === "Escape" && document.activeElement === q && q.value) {
      q.value = "";
      draw();
    }
  });

  // Coming back with the browser's back button can restore this page as it
  // was left, "Opening..." included.
  window.addEventListener("pageshow", function () {
    list.querySelectorAll(".opening").forEach(function (li) { li.classList.remove("opening"); });
  });

  version(home.version);
  agentFilter();
  draw();
  progress();
  poll();
})();
