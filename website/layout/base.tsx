import "../style.css";

interface NavItem {
  slug: string;
  title: string;
}

interface LayoutProps {
  children: React.ReactNode;
  nav?: NavItem[];
}

export default function Layout({ children, nav = [] }: LayoutProps) {
  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b border-border sticky top-0 bg-bg/80 backdrop-blur-sm z-50">
        <div className="max-w-4xl mx-auto px-6 h-14 flex items-center justify-between">
          <a
            href="/"
            className="text-fg font-semibold tracking-tight text-lg hover:opacity-80 transition-opacity"
          >
            bifrost
          </a>
          <nav className="flex items-center gap-6 text-sm">
            <a
              href="/docs/getting-started"
              className="text-muted hover:text-fg transition-colors"
            >
              Docs
            </a>
            <a
              href="https://github.com/3-lines-studio/bifrost"
              className="text-muted hover:text-fg transition-colors"
              target="_blank"
              rel="noopener noreferrer"
            >
              GitHub
            </a>
          </nav>
        </div>
      </header>

      <main className="flex-1">{children}</main>

      <footer className="border-t border-border">
        <div className="max-w-4xl mx-auto px-6 py-8 text-sm text-muted">
          <p>MIT License · Built with Bifrost</p>
        </div>
      </footer>
    </div>
  );
}
