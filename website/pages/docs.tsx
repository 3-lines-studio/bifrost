import Layout from "../layout/base";

interface NavItem {
  slug: string;
  title: string;
}

interface DocsProps {
  title: string;
  description: string;
  html: string;
  slug: string;
  nav: NavItem[];
}

export function Head({ title, description }: DocsProps) {
  return (
    <>
      <title>{`${title} — Bifrost`}</title>
      <meta
        name="description"
        content={description || `${title} — Bifrost documentation`}
      />
    </>
  );
}

export function Page({
  title,
  description,
  html,
  slug,
  nav,
}: DocsProps) {
  const currentIndex = nav.findIndex((item) => item.slug === slug);
  const prevItem = currentIndex > 0 ? nav[currentIndex - 1] : null;
  const nextItem =
    currentIndex < nav.length - 1 ? nav[currentIndex + 1] : null;

  return (
    <Layout nav={nav}>
      <div className="max-w-4xl mx-auto px-6 py-12 lg:grid lg:grid-cols-[200px_1fr] lg:gap-12">
        <aside className="hidden lg:block">
          <nav className="sticky top-20">
            <ul className="space-y-1 text-sm">
              {nav.map((item) => (
                <li key={item.slug}>
                  <a
                    href={`/docs/${item.slug}`}
                    className={`block py-1.5 transition-colors ${
                      item.slug === slug
                        ? "text-accent font-medium"
                        : "text-muted hover:text-fg"
                    }`}
                  >
                    {item.title}
                  </a>
                </li>
              ))}
            </ul>
          </nav>
        </aside>

        <article className="min-w-0">
          <div className="mb-8">
            <h1 className="text-3xl font-bold tracking-tight mb-2">{title}</h1>
            {description && (
              <p className="text-muted text-lg">{description}</p>
            )}
          </div>

          <div
            className="prose"
            dangerouslySetInnerHTML={{ __html: html }}
          />

          <div className="mt-12 pt-8 border-t border-border flex justify-between text-sm">
            {prevItem ? (
              <a
                href={`/docs/${prevItem.slug}`}
                className="text-muted hover:text-fg transition-colors"
              >
                ← {prevItem.title}
              </a>
            ) : (
              <span />
            )}
            {nextItem ? (
              <a
                href={`/docs/${nextItem.slug}`}
                className="text-muted hover:text-fg transition-colors"
              >
                {nextItem.title} →
              </a>
            ) : (
              <span />
            )}
          </div>
        </article>
      </div>
    </Layout>
  );
}
