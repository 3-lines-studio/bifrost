export function Hello({ name = "you" }: { name: string }) {
  return <div onClick={() => console.log("hello there")}>Hello, {name}!</div>;
}
