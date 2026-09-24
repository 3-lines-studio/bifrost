export default function Page() {
  return (
    <>
      <h2>Reload and downloads</h2>
      <p className="note" data-lab="reload">
        Normal links navigate in place. <a href="/labs/reload" data-bifrost-reload>Add data-bifrost-reload</a> to force a
        document load instead.
      </p>
      <ul className="cards">
        <li><a href="/labs/reload" data-bifrost-reload>data-bifrost-reload<small>document load, not a client navigation</small></a></li>
        <li><a href="/api/export" download="report.md">download<small>the download attribute is left alone</small></a></li>
        <li><a href="https://github.com/3-lines-studio/bifrost">external link<small>leaves the app</small></a></li>
      </ul>
    </>
  );
}
