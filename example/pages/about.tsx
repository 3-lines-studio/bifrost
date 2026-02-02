import Layout from "../layout/base";
import { themeScript } from "@/lib/theme-script";

export function Head() {
  return (
    <>
      <title>About</title>
      <script dangerouslySetInnerHTML={{ __html: themeScript }} />
    </>
  );
}

export function Page() {
  return <Layout>About me</Layout>;
}
