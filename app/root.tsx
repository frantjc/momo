import {
  isRouteErrorResponse,
  Links,
  Meta,
  Outlet,
  Scripts,
  ScrollRestoration,
  useRouteError,
} from "@remix-run/react";
import type { LinksFunction } from "@remix-run/node";

import styles from "./tailwind.css?url";

export const links: LinksFunction = () => [
  {
    rel: "stylesheet",
    href: styles,
  },
];

export function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <Meta />
        <Links />
      </head>
      <body>
        <div className="isolate">
          <div className="max-w-screen overflow-x-hidden">
            <header className="border-b border-gray-500">
              <nav className="flex h-14 items-center justify-between px-4">
                <a className="text-2xl font-bold hover:text-gray-500" aria-label="Home" href="/">Momo</a>
              </nav>
            </header>
            <main className="min-h-dvh container mx-auto px-2 tracking-wider">
              {children}
            </main>
          </div>
        </div>
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}

export default function Index() {
  return <Outlet />;
}

export function ErrorBoundary() {
  const err = useRouteError();

  return (
    <Layout>
      {
        isRouteErrorResponse(err)
          ? err.statusText
          : err instanceof Error
            ? err.message
            : "Unknown error"
      }
    </Layout>
  );
}
