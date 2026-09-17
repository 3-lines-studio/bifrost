import type { ReactNode } from "react";
import { MountProbe } from "../_components/mount-probe";

export default function Template({ children }: { children: ReactNode }) {
  return (
    <div data-template="labs">
      <MountProbe label="labs template" />
      {children}
    </div>
  );
}
