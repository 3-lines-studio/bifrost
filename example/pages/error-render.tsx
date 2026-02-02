import Layout from "@/layout/base";

export function Page() {
  throw new Error("This is a test render error");
  
  return (
    <Layout>
      <div>This will never render</div>
    </Layout>
  );
}
