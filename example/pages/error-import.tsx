import Layout from "@/layout/base";

throw new Error('Could not resolve: "invalid-import"');

export function Head() {
  return (
    <>
      <title>Error Import Test</title>
    </>
  );
}

export function Page() {
  return (
    <Layout>
      <div>This will never render</div>
    </Layout>
  );
}
