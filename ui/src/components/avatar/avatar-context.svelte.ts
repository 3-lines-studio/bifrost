import { setContext, getContext } from "svelte";

export type AvatarStatus = "loading" | "loaded" | "error";

export interface AvatarContext {
  get status(): AvatarStatus;
  setStatus(s: AvatarStatus): void;
}

const AVATAR_KEY = Symbol("avatar");

export function createAvatarContext(): void {
  let status = $state<AvatarStatus>("loading");
  setContext<AvatarContext>(AVATAR_KEY, {
    get status() {
      return status;
    },
    setStatus(s: AvatarStatus) {
      status = s;
    },
  });
}

export function getAvatarContext(): AvatarContext | undefined {
  return getContext<AvatarContext>(AVATAR_KEY);
}
