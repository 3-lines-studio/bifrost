import { formatCount } from "./lib/format.js";

export function mount(root) {
  const button = root.querySelector("[data-action='increment']");
  const output = root.querySelector("[data-count]");
  let count = 0;
  const onClick = () => {
    count += 1;
    output.textContent = formatCount(count);
  };
  button.addEventListener("click", onClick);
  return () => button.removeEventListener("click", onClick);
}
