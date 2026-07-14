import Layout from "@/layout/base";
import invalid from "invalid-import";

export function Head() {
  return (
    <>
      <title>Error Import Test</title>
    </>
  );
}

export function Page() {
  invalid.call();

  return (
    <Layout>
      <div>This will never render</div>
    </Layout>
  );
}
