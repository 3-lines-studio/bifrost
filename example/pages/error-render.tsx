import Layout from "@/layout/base";

export function Head() {
  return (
    <>
      <title>Error Render Test</title>
    </>
  );
}

export function Page() {
  throw new Error("This is a test render error");
  
  return (
    <Layout>
      <div>This will never render</div>
    </Layout>
  );
}
