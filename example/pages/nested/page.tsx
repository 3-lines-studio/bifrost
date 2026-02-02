import { Button } from "@/components/ui/button";
import { Hello } from "@/components/hello";
import Layout from "@/layout/base";
import { themeScript } from "@/lib/theme-script";

export function Head({ name }: { name: string }) {
  return (
    <>
      <title>{`Hello, ${name}`}</title>
      <meta name="description" content={`Hello ${name} from bifrost`} />
      <script dangerouslySetInnerHTML={{ __html: themeScript }} />
    </>
  );
}

export function Page({ name }: { name: string }) {
  return (
    <Layout>
      <Hello name={name} />
      <Button onClick={() => console.log("hello button")}>Nested</Button>
    </Layout>
  );
}
