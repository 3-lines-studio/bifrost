if (!window.__vanillaReady) {
  window.__vanillaReady = true;

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
