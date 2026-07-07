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
});
