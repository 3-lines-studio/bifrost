document.addEventListener("alpine:init", () => {
  Alpine.data("uiDialog", ({ id }) => ({
    id,
    open: false,
    show() {
      if (this.open) return;
      this.$refs.dialog.showModal();
      this.open = true;
    },
    close() {
      if (this.$refs.dialog.open) this.$refs.dialog.close();
      this.open = false;
    },
    closeOnBackdrop(event) {
      if (event.target === this.$refs.dialog) this.close();
    },
  }));

  Alpine.data("uiMenu", ({ id }) => ({
    id,
    open: false,
    active: -1,
    items() {
      return Array.from(this.$refs.menu.querySelectorAll('[role="menuitem"]'));
    },
    toggle() {
      if (this.open) {
        this.close();
        return;
      }
      this.openAndFocus();
    },
    openAndFocus() {
      this.open = true;
      this.$nextTick(() => this.items()[0]?.focus());
    },
    close(restoreFocus = true) {
      this.open = false;
      this.active = -1;
      if (!restoreFocus) return;
      const trigger = this.$refs.trigger;
      const button = trigger?.querySelector?.("button") ?? trigger;
      button?.focus();
    },
    focus(index) {
      const items = this.items();
      if (items.length === 0) return;
      this.active = (index + items.length) % items.length;
      items[this.active].focus();
    },
    onKeydown(event) {
      const items = this.items();
      if (items.length === 0) return;
      const current = items.indexOf(document.activeElement);
      switch (event.key) {
        case "ArrowDown":
          event.preventDefault();
          this.focus(current + 1);
          break;
        case "ArrowUp":
          event.preventDefault();
          this.focus(current - 1);
          break;
        case "Home":
          event.preventDefault();
          this.focus(0);
          break;
        case "End":
          event.preventDefault();
          this.focus(items.length - 1);
          break;
        case "Escape":
          event.preventDefault();
          this.close();
          break;
        case "Tab":
          this.close();
          break;
        default:
          if (event.key.length === 1) {
            const match = items.find((item) =>
              item.textContent.trim().toLowerCase().startsWith(event.key.toLowerCase()),
            );
            if (match) {
              event.preventDefault();
              match.focus();
            }
          }
      }
    },
  }));

  Alpine.data("uiTabs", ({ id, value }) => ({
    id,
    value,
    select(next) {
      this.value = next;
    },
    onKeydown(event) {
      const tabs = Array.from(this.$refs.list.querySelectorAll('[role="tab"]'));
      if (tabs.length === 0) return;
      const current = tabs.indexOf(document.activeElement);
      if (current < 0) return;
      let next = current;
      switch (event.key) {
        case "ArrowRight":
          next = (current + 1) % tabs.length;
          break;
        case "ArrowLeft":
          next = (current - 1 + tabs.length) % tabs.length;
          break;
        case "Home":
          next = 0;
          break;
        case "End":
          next = tabs.length - 1;
          break;
        default:
          return;
      }
      event.preventDefault();
      tabs[next].focus();
      tabs[next].click();
    },
  }));

  Alpine.store("toasts", {
    items: [],
    sequence: 0,
    push(message, type = "info") {
      const id = ++this.sequence;
      this.items.push({ id, message, type });
      window.setTimeout(() => this.remove(id), 4000);
    },
    remove(id) {
      this.items = this.items.filter((toast) => toast.id !== id);
    },
  });
});
