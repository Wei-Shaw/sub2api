document.addEventListener('DOMContentLoaded', function () {
  // Mobile menu toggle
  const menuBtn = document.querySelector('.mobile-menu-btn');
  const nav = document.querySelector('.nav');
  if (menuBtn && nav) {
    menuBtn.addEventListener('click', function () {
      nav.classList.toggle('open');
      const icon = menuBtn.querySelector('svg use');
      if (icon) {
        const isOpen = nav.classList.contains('open');
        menuBtn.setAttribute('aria-expanded', isOpen);
      }
    });
  }

  // Active nav link
  const currentPath = window.location.pathname.replace(/\.html$/, '').replace(/\/$/, '') || '/';
  document.querySelectorAll('.nav a').forEach(function (link) {
    const href = link.getAttribute('href').replace(/\.html$/, '').replace(/\/$/, '') || '/';
    if (href === currentPath) {
      link.classList.add('active');
    }
  });

  // Smooth scroll for docs sidebar
  document.querySelectorAll('.docs-sidebar-group a[href^="#"]').forEach(function (link) {
    link.addEventListener('click', function (e) {
      const target = document.querySelector(this.getAttribute('href'));
      if (target) {
        e.preventDefault();
        target.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
    });
  });

  initButterfly();
});

// Living specimen — a butterfly takes off from the hero eye, darts up to the
// wordmark, flaps twice, and stays perched there (on every page).
function initButterfly() {
  var logo = document.querySelector('.logo');
  var logoText = document.querySelector('.logo-text');
  if (!logo || !logoText) return;

  var reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  var fontsReady = (document.fonts && document.fonts.ready) ? document.fonts.ready : Promise.resolve();

  var bfly = document.createElement('div');
  bfly.className = 'bfly';
  bfly.setAttribute('aria-hidden', 'true');
  bfly.innerHTML =
    '<div class="bfly-inner">' +
    '<i class="bfly-wing bfly-wing--l"></i>' +
    '<i class="bfly-wing bfly-wing--r"></i>' +
    '</div>';
  document.body.appendChild(bfly);
  var wingL = bfly.querySelector('.bfly-wing--l');

  // one-shot wing animations driven by CSS classes
  function pulse(cls) {
    if (reduceMotion || bfly.classList.contains(cls)) return;
    bfly.classList.add(cls);
    wingL.addEventListener('animationend', function () { bfly.classList.remove(cls); }, { once: true });
  }

  // butterfly center point when perched: astride the final "e" of the wordmark
  function perchTarget() {
    var r = logoText.getBoundingClientRect();
    return { x: r.right - bfly.offsetWidth * 0.06, y: r.top - bfly.offsetHeight * 0.08 };
  }

  function placePerch() {
    var logoRect = logo.getBoundingClientRect();
    var t = perchTarget();
    bfly.style.left = (t.x - logoRect.left - bfly.offsetWidth / 2) + 'px';
    bfly.style.top = (t.y - logoRect.top - bfly.offsetHeight / 2) + 'px';
  }

  function perch() {
    logo.style.position = 'relative';
    logo.appendChild(bfly);
    bfly.classList.remove('is-flying');
    bfly.classList.add('is-active', 'is-perched');
    bfly.style.transform = '';
    placePerch();
  }

  function scheduleIdleFlap() {
    if (reduceMotion) return;
    setTimeout(function () {
      pulse('is-flap');
      scheduleIdleFlap();
    }, 8000 + Math.random() * 7000);
  }

  window.addEventListener('resize', function () {
    if (bfly.classList.contains('is-perched')) placePerch();
    else if (sitting) placeSit();
  });
  logo.addEventListener('mouseenter', function () {
    if (bfly.classList.contains('is-perched')) pulse('is-greet');
  });

  function cubicAt(p0, p1, p2, p3, t) {
    var u = 1 - t;
    return {
      x: u * u * u * p0.x + 3 * u * u * t * p1.x + 3 * u * t * t * p2.x + t * t * t * p3.x,
      y: u * u * u * p0.y + 3 * u * u * t * p1.y + 3 * u * t * t * p2.y + t * t * t * p3.y
    };
  }
  function cubicTangent(p0, p1, p2, p3, t) {
    var u = 1 - t;
    return {
      x: 3 * u * u * (p1.x - p0.x) + 6 * u * t * (p2.x - p1.x) + 3 * t * t * (p3.x - p2.x),
      y: 3 * u * u * (p1.y - p0.y) + 6 * u * t * (p2.y - p1.y) + 3 * t * t * (p3.y - p2.y)
    };
  }

  // where it sits before takeoff: anchored to the hero eye, so it follows scroll
  function sitAnchor() {
    var fig = document.querySelector('.hero-figure img');
    if (fig) {
      var r = fig.getBoundingClientRect();
      return { x: r.left + r.width * 0.5, y: r.top + r.height * 0.4 };
    }
    return {
      x: Math.max(60, Math.min(window.innerWidth * 0.85, window.innerWidth - 60)),
      y: Math.min(window.innerHeight * 0.7, window.innerHeight - 70)
    };
  }

  function placeSit() {
    var p = sitAnchor();
    var w = bfly.offsetWidth, h = bfly.offsetHeight;
    bfly.style.transform = 'translate(' + (p.x - w / 2).toFixed(1) + 'px,' +
      (p.y - h / 2).toFixed(1) + 'px) rotate(4deg)';
  }

  var FLIGHT_MS = 2250;
  var BEAT_MS = 220; // must match the CSS wingbeat period
  var T_CRUISE = 0.78; // cruise arrives at the hover point…
  var T_DOWN = 0.86;   // …lingers there, then touches down

  // sampled flight: near-vertical burst off the eye, undulating cruise,
  // a brief hover beside the wordmark, then a soft touchdown
  function buildFrames(start) {
    var w = bfly.offsetWidth, h = bfly.offsetHeight;
    var target = perchTarget();
    var hover = { x: target.x + w * 0.5, y: target.y + h * 0.55 };

    var dx = Math.abs(hover.x - start.x);
    var dy = Math.abs(hover.y - start.y);
    // p1 above the start makes it leave vertically; p2 brings it in from the right
    var p1 = { x: start.x - 24, y: start.y - Math.max(150, dy * 0.5) };
    var p2 = { x: hover.x + Math.max(130, dx * 0.3), y: hover.y - 36 };

    var beats = FLIGHT_MS / BEAT_MS;
    var N = 72, frames = [];
    for (var i = 0; i <= N; i++) {
      var t = i / N;
      var ct = Math.min(1, t / T_CRUISE);
      var s = 1 - Math.pow(1 - ct, 1.9); // darts off, decelerates into the hover
      var pos = cubicAt(start, p1, p2, hover, s);

      // slow undulation while cruising, fading out on arrival
      pos.y += Math.sin(t * Math.PI * 2.3 + 0.6) * 11 * (1 - s);

      // touchdown progress
      var d = t > T_DOWN ? (t - T_DOWN) / (1 - T_DOWN) : 0;
      d = d * d * (3 - 2 * d);
      if (t > T_CRUISE) {
        var beatPhase = t * beats * Math.PI * 2;
        pos.x = hover.x * (1 - d) + target.x * d + Math.sin(beatPhase) * 2.5 * (1 - d);
        pos.y = hover.y * (1 - d) + target.y * d + Math.cos(beatPhase) * 2 * (1 - d);
      }

      var dt = cubicTangent(start, p1, p2, hover, s);
      var bank = Math.atan2(dt.x, -dt.y) * 180 / Math.PI; // 0deg = heading straight up
      if (bank > 180) bank -= 360;
      if (bank < -180) bank += 360;
      bank = Math.max(-38, Math.min(38, bank * 0.4));
      var rot = t <= T_CRUISE
        ? bank
        : bank * (1 - d) + (-10) * d + Math.sin(t * beats * Math.PI * 2) * 3 * (1 - d);

      var scale = t <= T_CRUISE
        ? 1 + 0.36 * Math.sin(Math.PI * s) // rises toward the viewer mid-flight
        : 1 - 0.18 * d;                    // settles down to perch size

      frames.push({
        transform: 'translate(' + (pos.x - w / 2).toFixed(1) + 'px,' + (pos.y - h / 2).toFixed(1) + 'px)' +
          ' rotate(' + rot.toFixed(1) + 'deg) scale(' + scale.toFixed(3) + ')'
      });
    }
    return frames;
  }

  var appearAnim = null;
  var sitting = false, flown = false;
  var flapTimer = null, flightTimer = null, sitScrollBase = 0;

  // fade in seated on the hero eye, gather with one slow flap, then burst off
  function preflight() {
    sitting = true;
    sitScrollBase = window.scrollY;
    bfly.classList.add('is-active');
    placeSit();
    appearAnim = bfly.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 400, easing: 'ease-out', fill: 'forwards' });
    flapTimer = setTimeout(function () { pulse('is-flap'); }, 450);
    flightTimer = setTimeout(fly, 1600);
  }

  // while seated it rides the page; a real scroll startles it into flight
  window.addEventListener('scroll', function () {
    if (!sitting) return;
    placeSit();
    if (Math.abs(window.scrollY - sitScrollBase) > 30) fly();
  }, { passive: true });

  function fly() {
    if (flown) return;
    flown = true;
    sitting = false;
    clearTimeout(flapTimer);
    clearTimeout(flightTimer);
    bfly.classList.remove('is-flap');
    bfly.classList.add('is-active', 'is-flying');
    var frames = buildFrames(sitAnchor());
    bfly.style.transform = frames[0].transform;
    var anim = bfly.animate(frames, { duration: FLIGHT_MS, easing: 'linear', fill: 'forwards' });
    anim.onfinish = function () {
      perch();
      anim.cancel();
      if (appearAnim) appearAnim.cancel();
      pulse('is-settling');
      scheduleIdleFlap();
    };
  }

  // ?bflyT=0.6 freezes the flight at that point (visual debugging)
  var dbg = location.search.match(/[?&]bflyT=([0-9.]+)/);

  var isHome = !!document.querySelector('.hero-figure');
  fontsReady.then(function () {
    if (dbg) {
      var dt = parseFloat(dbg[1]);
      if (dt >= 1) { perch(); return; }
      bfly.classList.add('is-active', 'is-flying');
      var frames = buildFrames(sitAnchor());
      bfly.style.transform = frames[Math.round(dt * (frames.length - 1))].transform;
      return;
    }
    if (!isHome || reduceMotion || !bfly.animate) {
      // inner pages & reduced motion: it already lives here
      requestAnimationFrame(perch);
      if (!reduceMotion) scheduleIdleFlap();
      return;
    }
    // let the hero settle before the butterfly appears
    var takeoff = function () { setTimeout(preflight, 500); };
    if (document.readyState === 'complete') takeoff();
    else window.addEventListener('load', takeoff, { once: true });
  });
}
