import "./island.css";
import { stamp } from "./format";

const button = document.querySelector<HTMLButtonElement>("#island-button");
const output = document.querySelector<HTMLElement>("#island-output");

if (button && output) {
  button.addEventListener("click", () => {
    output.textContent = stamp(new Date());
  });
}
