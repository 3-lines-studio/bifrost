type Row = { id: string; cells: string[] };

export function Head({ title }: { title: string }) {
  return (
    <>
      <title>{title}</title>
      <meta name="description" content="Bifrost heavy SSR bench page" />
    </>
  );
}

const cellStyle: React.CSSProperties = {
  padding: "2px 6px",
  border: "1px solid #ddd",
  fontSize: "12px",
  whiteSpace: "nowrap",
};

export function Page({ title, rows }: { title: string; rows: Row[] }) {
  return (
    <div style={{ fontFamily: "system-ui, sans-serif", margin: "0 auto", maxWidth: 1400 }}>
      <h1>{title}</h1>
      <p>
        {rows.length} rows, rendered server-side.
      </p>
      <table style={{ borderCollapse: "collapse", width: "100%" }}>
        <thead>
          <tr>
            {rows[0]?.cells.map((_, i) => (
              <th key={i} style={cellStyle}>
                col-{i}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id}>
              {row.cells.map((cell, i) => (
                <td key={i} style={cellStyle}>
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
