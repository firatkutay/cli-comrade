/* ============================================================
   cli-comrade landing — behavior
   Vanilla JS, no dependencies.
   ============================================================ */
(function () {
  "use strict";

  var prefersReduced =
    window.matchMedia &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* -----------------------------------------------------------
     LANGUAGE
     ----------------------------------------------------------- */
  var STORE_KEY = "cli-comrade-lang";
  var lang = detectLang();

  function detectLang() {
    var stored = null;
    try { stored = localStorage.getItem(STORE_KEY); } catch (e) {}
    if (stored === "tr" || stored === "en") return stored;
    var nav = (navigator.language || navigator.userLanguage || "en").toLowerCase();
    return nav.indexOf("tr") === 0 ? "tr" : "en";
  }

  function applyLang() {
    document.documentElement.lang = lang;

    // textContent swaps
    var nodes = document.querySelectorAll("[data-tr][data-en]");
    for (var i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      var val = lang === "tr" ? el.getAttribute("data-tr") : el.getAttribute("data-en");
      if (val !== null) el.textContent = val;
    }

    // aria-label swaps
    var ariaNodes = document.querySelectorAll("[data-tr-aria][data-en-aria]");
    for (var j = 0; j < ariaNodes.length; j++) {
      var a = ariaNodes[j];
      a.setAttribute("aria-label", lang === "tr" ? a.getAttribute("data-tr-aria") : a.getAttribute("data-en-aria"));
    }

    // safety rule (contains inline code emphasis)
    renderSafetyRule();

    // language toggle pressed state
    var btns = document.querySelectorAll(".langtoggle__btn");
    for (var k = 0; k < btns.length; k++) {
      var pressed = btns[k].getAttribute("data-lang") === lang;
      btns[k].setAttribute("aria-pressed", pressed ? "true" : "false");
    }

    // copy button labels (respect "copied" state)
    var copies = document.querySelectorAll(".copy");
    for (var c = 0; c < copies.length; c++) setCopyLabel(copies[c]);

    // restart terminal in the new language
    startTerminal();
  }

  function setLang(next) {
    if (next === lang) return;
    lang = next;
    try { localStorage.setItem(STORE_KEY, lang); } catch (e) {}
    applyLang();
  }

  var langToggle = document.querySelector(".langtoggle");
  if (langToggle) {
    langToggle.addEventListener("click", function (e) {
      var btn = e.target.closest(".langtoggle__btn");
      if (btn) setLang(btn.getAttribute("data-lang"));
    });
  }

  /* safety rule with emphasized `code` fragments (kept in JS to allow markup) */
  function renderSafetyRule() {
    var el = document.querySelector(".safety__rule");
    if (!el) return;
    if (lang === "tr") {
      el.innerHTML =
        "<b>auto</b> modda bile yıkıcı komutlar (<code>rm -rf /</code>, <code>mkfs</code>, <code>dd</code>, fork-bomb…) daima onay ister. Yerel bir kural motoru + denylist ikinci kontrolü yapar ve LLM'e asla güvenmez.";
    } else {
      el.innerHTML =
        "Even in <b>auto</b>, destructive commands (<code>rm -rf /</code>, <code>mkfs</code>, <code>dd</code>, fork bombs…) always require confirmation. A local rule engine + denylist does a second check and never trusts the LLM.";
    }
  }

  /* -----------------------------------------------------------
     COPY BUTTONS
     ----------------------------------------------------------- */
  function setCopyLabel(btn) {
    var span = btn.querySelector(".copy-label");
    if (!span) return;
    if (btn.classList.contains("soon")) {
      span.textContent = lang === "tr" ? "Yakında" : "Soon";
    } else if (btn.classList.contains("copied")) {
      span.textContent = lang === "tr" ? "Kopyalandı ✓" : "Copied ✓";
    } else {
      span.textContent = lang === "tr" ? "Kopyala" : "Copy";
    }
  }

  var copyButtons = document.querySelectorAll(".copy");
  for (var ci = 0; ci < copyButtons.length; ci++) {
    (function (btn) {
      var text = btn.getAttribute("data-copy") || "";
      var soon = btn.getAttribute("data-soon") === "1";
      btn.setAttribute("aria-label", soon ? "Yakında / Soon" : "Kopyala / Copy");
      btn.addEventListener("click", function () {
        // Channels not live yet (winget/snap): don't copy — flash a red "Soon".
        if (soon) {
          btn.classList.add("soon");
          setCopyLabel(btn);
          window.clearTimeout(btn._t);
          btn._t = window.setTimeout(function () {
            btn.classList.remove("soon");
            setCopyLabel(btn);
          }, 1800);
          return;
        }
        copyText(text).then(function () {
          btn.classList.add("copied");
          setCopyLabel(btn);
          window.clearTimeout(btn._t);
          btn._t = window.setTimeout(function () {
            btn.classList.remove("copied");
            setCopyLabel(btn);
          }, 1800);
        });
      });
    })(copyButtons[ci]);
  }

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).catch(function () {
        return legacyCopy(text);
      });
    }
    return legacyCopy(text);
  }
  function legacyCopy(text) {
    return new Promise(function (resolve) {
      var ta = document.createElement("textarea");
      ta.value = text;
      ta.setAttribute("readonly", "");
      ta.style.position = "absolute";
      ta.style.left = "-9999px";
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand("copy"); } catch (e) {}
      document.body.removeChild(ta);
      resolve();
    });
  }

  /* -----------------------------------------------------------
     INSTALL TABS (ARIA tablist + keyboard)
     ----------------------------------------------------------- */
  var tabs = Array.prototype.slice.call(document.querySelectorAll(".tab"));
  function selectTab(tab) {
    for (var i = 0; i < tabs.length; i++) {
      var t = tabs[i];
      var on = t === tab;
      t.setAttribute("aria-selected", on ? "true" : "false");
      t.tabIndex = on ? 0 : -1;
      var panel = document.getElementById(t.getAttribute("aria-controls"));
      if (panel) panel.hidden = !on;
    }
  }
  tabs.forEach(function (tab, idx) {
    tab.addEventListener("click", function () { selectTab(tab); });
    tab.addEventListener("keydown", function (e) {
      var next = null;
      if (e.key === "ArrowRight" || e.key === "ArrowDown") next = tabs[(idx + 1) % tabs.length];
      else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = tabs[(idx - 1 + tabs.length) % tabs.length];
      else if (e.key === "Home") next = tabs[0];
      else if (e.key === "End") next = tabs[tabs.length - 1];
      if (next) { e.preventDefault(); selectTab(next); next.focus(); }
    });
  });

  /* -----------------------------------------------------------
     3D PARALLAX on the CRT (pointer-driven, small angles)
     ----------------------------------------------------------- */
  var stage = document.getElementById("crtStage");
  var crt = document.getElementById("crt");
  var canParallax =
    !prefersReduced &&
    stage &&
    crt &&
    window.matchMedia &&
    window.matchMedia("(min-width: 861px) and (pointer: fine)").matches;

  if (canParallax) {
    var baseX = 5, baseY = -9, maxX = 6, maxY = 8;
    stage.addEventListener("mousemove", function (e) {
      var r = stage.getBoundingClientRect();
      var px = (e.clientX - r.left) / r.width - 0.5;
      var py = (e.clientY - r.top) / r.height - 0.5;
      var rotY = baseY + px * maxY * 2;
      var rotX = baseX - py * maxX * 2;
      crt.style.transform = "rotateY(" + rotY.toFixed(2) + "deg) rotateX(" + rotX.toFixed(2) + "deg)";
    });
    stage.addEventListener("mouseleave", function () {
      crt.style.transform = "rotateY(" + baseY + "deg) rotateX(" + baseX + "deg)";
    });
  }

  /* -----------------------------------------------------------
     SIGNATURE TERMINAL — typewriter (bilingual, colored, a11y)
     ----------------------------------------------------------- */
  var out = document.getElementById("crtOut");

  // Sessions: array of tokens {t: text, c: className (""=phosphor green), hold: ms after}
  function session(l) {
    var legend = l === "tr"
      ? "[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü: "
      : "[y]es [n]o [e]dit [x]plain [a]ll: ";
    if (l === "tr") {
      return [
        { t: "$ ", c: "t-prompt" },
        { t: "comrade ", c: "t-user" },
        { t: "\"8080 portunu kim kullanıyor bul ve kapat\"\n", c: "t-user", hold: 600 },
        { t: "  comrade:", c: "t-label" },
        { t: " 8080'i dinleyen süreci bulup sonlandıracağım.\n", c: "", hold: 380 },
        { t: "  ", c: "" }, { t: "→ ", c: "t-arrow" }, { t: "lsof -ti:8080\n", c: "t-cmd", hold: 260 },
        { t: "  ", c: "" }, { t: "[read]", c: "t-read" }, { t: "  ", c: "" },
        { t: legend, c: "t-legend", hold: 620 },
        { t: "e\n", c: "t-answer", hold: 420 },
        { t: "  ", c: "" }, { t: "✓", c: "t-ok" },
        { t: " süreç bulundu: node (pid 4213)\n", c: "", hold: 360 },
        { t: "  ", c: "" }, { t: "→ ", c: "t-arrow" }, { t: "kill 4213\n", c: "t-cmd", hold: 260 },
        { t: "  ", c: "" }, { t: "[destructive]", c: "t-destructive" }, { t: "  ", c: "" },
        { t: legend, c: "t-legend", hold: 2600 }
      ];
    }
    return [
      { t: "$ ", c: "t-prompt" },
      { t: "comrade ", c: "t-user" },
      { t: "\"find what's using port 8080 and kill it\"\n", c: "t-user", hold: 600 },
      { t: "  comrade:", c: "t-label" },
      { t: " I'll find the process listening on 8080 and stop it.\n", c: "", hold: 380 },
      { t: "  ", c: "" }, { t: "→ ", c: "t-arrow" }, { t: "lsof -ti:8080\n", c: "t-cmd", hold: 260 },
      { t: "  ", c: "" }, { t: "[read]", c: "t-read" }, { t: "  ", c: "" },
      { t: legend, c: "t-legend", hold: 620 },
      { t: "y\n", c: "t-answer", hold: 420 },
      { t: "  ", c: "" }, { t: "✓", c: "t-ok" },
      { t: " process found: node (pid 4213)\n", c: "", hold: 360 },
      { t: "  ", c: "" }, { t: "→ ", c: "t-arrow" }, { t: "kill 4213\n", c: "t-cmd", hold: 260 },
      { t: "  ", c: "" }, { t: "[destructive]", c: "t-destructive" }, { t: "  ", c: "" },
      { t: legend, c: "t-legend", hold: 2600 }
    ];
  }

  // Flatten tokens into a per-character array, tracking class + hold-after positions.
  function flatten(tokens) {
    var chars = [];
    var holds = {}; // char index (1-based count) -> ms hold after typing it
    for (var i = 0; i < tokens.length; i++) {
      var tk = tokens[i];
      for (var n = 0; n < tk.t.length; n++) chars.push({ ch: tk.t[n], c: tk.c });
      if (tk.hold) holds[chars.length] = tk.hold; // after this many chars, hold
    }
    return { chars: chars, holds: holds };
  }

  function escapeHTML(s) {
    return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  // Render first `count` chars as grouped colored spans + trailing cursor.
  function render(chars, count) {
    var html = "";
    var runCls = null;
    var runTxt = "";
    function flush() {
      if (runTxt === "") return;
      if (runCls) html += '<span class="' + runCls + '">' + escapeHTML(runTxt) + "</span>";
      else html += escapeHTML(runTxt);
      runTxt = "";
    }
    for (var i = 0; i < count; i++) {
      var c = chars[i];
      if (c.c !== runCls) { flush(); runCls = c.c; }
      runTxt += c.ch;
    }
    flush();
    html += '<span class="crt__cursor" aria-hidden="true"></span>';
    out.innerHTML = html;
  }

  var termTimer = null;

  function startTerminal() {
    if (!out) return;
    if (termTimer) { window.clearTimeout(termTimer); termTimer = null; }

    var data = flatten(session(lang));
    var chars = data.chars;
    var holds = data.holds;

    if (prefersReduced) {
      render(chars, chars.length); // freeze at final complete frame
      return;
    }

    var idx = 0;
    function tick() {
      idx++;
      render(chars, idx);
      if (idx >= chars.length) {
        termTimer = window.setTimeout(function () {
          idx = 0;
          render(chars, 0);
          termTimer = window.setTimeout(tick, 500);
        }, 2600);
        return;
      }
      var delay = 26 + Math.random() * 34;
      if (holds[idx]) delay += holds[idx];
      termTimer = window.setTimeout(tick, delay);
    }
    render(chars, 0);
    termTimer = window.setTimeout(tick, 450);
  }

  /* -----------------------------------------------------------
     SCROLL REVEAL — staggered rise + fade (IntersectionObserver)
     Progressive enhancement: without JS (or under reduced motion)
     all content stays fully visible; we only add hidden state here.
     ----------------------------------------------------------- */
  function setupReveal() {
    if (prefersReduced || !("IntersectionObserver" in window)) return;
    document.body.classList.add("reveals-enabled");

    var io = new IntersectionObserver(
      function (entries) {
        for (var i = 0; i < entries.length; i++) {
          var en = entries[i];
          if (!en.isIntersecting) continue;
          var el = en.target;
          el.classList.add("is-visible");
          io.unobserve(el);
          // After the reveal settles, drop the reveal wiring so the
          // element's own (fast) hover transition takes back over.
          (function (node) {
            window.setTimeout(function () {
              node.classList.remove("reveal");
              node.style.transitionDelay = "";
              node.style.willChange = "";
            }, 720);
          })(el);
        }
      },
      { threshold: 0.12, rootMargin: "0px 0px -6% 0px" }
    );

    function observe(el, delayMs) {
      el.classList.add("reveal");
      if (delayMs) el.style.transitionDelay = delayMs + "ms";
      io.observe(el);
    }

    // Section headers rise on their own.
    var solos = document.querySelectorAll(
      ".section .eyebrow, .section .section__title, .install__after"
    );
    for (var s = 0; s < solos.length; s++) observe(solos[s], 0);

    // Grids stagger their children left-to-right.
    [".cards", ".modes", ".providers"].forEach(function (sel) {
      var wrap = document.querySelector(sel);
      if (!wrap) return;
      var kids = wrap.children;
      for (var k = 0; k < kids.length; k++) observe(kids[k], k * 70);
    });

    // Safety two-column block.
    var safety = document.querySelectorAll(".safety__demo, .safety__copy");
    for (var f = 0; f < safety.length; f++) observe(safety[f], f * 80);

    // Install controls.
    var tablist = document.querySelector(".install__tabs");
    if (tablist) observe(tablist, 0);
    var panels = document.querySelector(".install__panels");
    if (panels) observe(panels, 90);
  }

  /* -----------------------------------------------------------
     SCROLL PARALLAX — continuous depth as you scroll up/down.
     Uses the standalone `translate` property (NOT `transform`) so it
     composes with the CRT 3D tilt and the reveal transform without
     conflict. Bases are measured once (stable, document-space), so
     update() never reads back its own applied translate — no feedback
     loop / jitter. Fully disabled under reduced motion.
     ----------------------------------------------------------- */
  function setupParallax() {
    if (prefersReduced || !("requestAnimationFrame" in window)) return;
    if (!(window.CSS && CSS.supports && CSS.supports("translate", "0px 1px"))) return;

    var layers = [];
    function add(sel, speed) {
      var els = document.querySelectorAll(sel);
      for (var i = 0; i < els.length; i++) layers.push({ el: els[i], speed: speed, base: 0 });
    }
    add(".hero__logo", 0.08);
    add(".crt-stage", -0.06);
    add(".section .eyebrow", 0.1);
    add(".section .section__title", 0.05);
    if (!layers.length) return;

    function measure() {
      var sy = window.pageYOffset || document.documentElement.scrollTop;
      var i;
      for (i = 0; i < layers.length; i++) layers[i].el.style.translate = ""; // clear to read true layout
      for (i = 0; i < layers.length; i++) {
        var r = layers[i].el.getBoundingClientRect();
        layers[i].base = r.top + sy + r.height / 2; // stable document-space center
        layers[i].el.style.willChange = "translate";
      }
    }
    var ticking = false;
    function update() {
      ticking = false;
      var mid = (window.pageYOffset || document.documentElement.scrollTop) + window.innerHeight / 2;
      for (var i = 0; i < layers.length; i++) {
        var L = layers[i];
        L.el.style.translate = "0px " + ((mid - L.base) * L.speed).toFixed(1) + "px";
      }
    }
    function onScroll() {
      if (!ticking) { ticking = true; window.requestAnimationFrame(update); }
    }
    measure();
    update();
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", function () { measure(); update(); }, { passive: true });
  }

  /* -----------------------------------------------------------
     HEADER SHRINK — collapse the sticky top bar once you scroll
     off the very top. Independent of parallax so it still works
     under reduced motion (the height change just becomes instant).
     ----------------------------------------------------------- */
  function setupHeaderShrink() {
    var bar = document.querySelector(".topbar");
    if (!bar) return;
    var ticking = false;
    function apply() {
      ticking = false;
      var sy = window.pageYOffset || document.documentElement.scrollTop;
      bar.classList.toggle("topbar--scrolled", sy > 24);
    }
    function onScroll() {
      if (!ticking) { ticking = true; window.requestAnimationFrame(apply); }
    }
    apply(); // initial state (handles reload while already scrolled)
    window.addEventListener("scroll", onScroll, { passive: true });
  }

  /* -----------------------------------------------------------
     INIT
     ----------------------------------------------------------- */
  applyLang(); // also starts the terminal
  setupReveal();
  setupParallax();
  setupHeaderShrink();
})();
