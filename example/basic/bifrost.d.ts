declare module "virtual:bifrost/routes" {
  export interface BifrostRoute {
    pattern: string;
    view: string;
    kind: "server" | "static" | "client";
  }

  export const routes: BifrostRoute[];

  export function href(pattern: string, params?: Record<string, string | string[]>): string;
}
