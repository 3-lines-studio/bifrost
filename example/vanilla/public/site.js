if (!window.__vanillaReady) {
  window.__vanillaReady = true;

  document.addEventListener("turbo:load", () => {
    document.querySelectorAll("[data-nav]").forEach((link) => {
      if (link.getAttribute("href") === location.pathname) {
        link.setAttribute("aria-current", "page");
      } else {
        link.removeAttribute("aria-current");
      }
    });
    const boot = document.getElementById("boot");
    if (boot) {
      boot.textContent = "Turbo + Alpine";
    }
  });

  document.addEventListener("alpine:init", () => {
    Alpine.data("clock", () => ({
      time: "",
      busy: false,
      async load() {
        this.busy = true;
        try {
          const response = await fetch("/api/time", { headers: { Accept: "application/json" } });
          if (response.ok) {
            this.time = (await response.json()).time;
          }
        } finally {
          this.busy = false;
        }
      },
    }));
  });
}
