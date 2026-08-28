import { FormEvent, useEffect, useState } from "react";
import { GreetService } from "../bindings/github.com/3-lines-studio/bifrost/example/wails";
import "./style.css";

export function Page() {
  const [name, setName] = useState("");
  const [message, setMessage] = useState("Call Go through a generated Wails binding.");
  const [platform, setPlatform] = useState("");
  const [path, setPath] = useState(window.location.pathname);

  useEffect(() => {
    const updatePath = () => {
      setPath(window.location.pathname);
    };
    window.addEventListener("popstate", updatePath);
    return () => {
      window.removeEventListener("popstate", updatePath);
    };
  }, []);

  const navigate = (nextPath: string) => {
    window.history.pushState({}, "", nextPath);
    setPath(nextPath);
  };

  const greet = async (event: FormEvent) => {
    event.preventDefault();
    try {
      const greeting = await GreetService.Greet(name);
      setMessage(greeting.message);
      setPlatform(greeting.platform);
    } catch (error) {
      setMessage(String(error));
      setPlatform("");
    }
  };

  return (
    <main>
      <section className="hero">
        <div className="eyebrow">Bifrost + Wails</div>
        <h1>One React client, one Go application.</h1>
        <p>Bifrost builds the frontend. Wails owns the window, bindings, and native package.</p>
      </section>

      <nav aria-label="Application pages">
        <button className={path === "/" ? "active" : ""} onClick={() => navigate("/")} type="button">
          Home
        </button>
        <button className={path === "/settings" ? "active" : ""} onClick={() => navigate("/settings")} type="button">
          Settings
        </button>
      </nav>

      <section className="card">
        <span className="route">Current route: {path}</span>
        {path === "/settings" ? (
          <div>
            <h2>Client route</h2>
            <p>The catch-all Bifrost route mounts this same application after a direct reload.</p>
          </div>
        ) : (
          <form onSubmit={greet}>
            <label htmlFor="name">Name</label>
            <div className="form-row">
              <input id="name" onChange={(event) => setName(event.target.value)} placeholder="Don Berti" value={name} />
              <button type="submit">Call Go</button>
            </div>
            <output>
              <strong>{message}</strong>
              {platform ? <span>{platform}</span> : null}
            </output>
          </form>
        )}
      </section>
    </main>
  );
}
