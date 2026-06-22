import { setContext, getContext } from "svelte";

export type ModuleStatus = "idle" | "active";

export interface ModuleContext {
  readonly status: ModuleStatus;
}

const MODULE_KEY = Symbol("module-context");

export function createModuleContext(): void {
  let status = $state<ModuleStatus>("idle");
  setContext<ModuleContext>(MODULE_KEY, {
    get status() {
      return status;
    },
  });
}

export function getModuleContext(): ModuleContext | undefined {
  return getContext<ModuleContext>(MODULE_KEY);
}
