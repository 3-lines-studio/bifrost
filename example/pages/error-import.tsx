import Layout from "@/layout/base";
import invalid from "invalid-import";

export function Page() {
  invalid.call();

  return (
    <Layout>
      <div>This will never render</div>
    </Layout>
  );
}
