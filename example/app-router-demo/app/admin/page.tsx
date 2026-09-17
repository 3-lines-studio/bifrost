export default function Page(props: { middleware: string[]; cookie: boolean; pathname: string }) {
  return (
    <table className="data" data-admin="overview">
      <tbody>
        <tr><th>middleware order</th><td>{props.middleware.join(" → ") || "none"}</td></tr>
        <tr><th>session cookie</th><td>{props.cookie ? "present" : "missing"}</td></tr>
        <tr><th>pathname</th><td>{props.pathname}</td></tr>
      </tbody>
    </table>
  );
}
