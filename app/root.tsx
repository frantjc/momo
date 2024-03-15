import { ChakraProvider } from "@chakra-ui/react";
import { withEmotionCache } from "@emotion/react";
import {
  Links,
  LiveReload,
  Meta,
  Outlet,
  Scripts,
  ScrollRestoration,
} from "@remix-run/react";
import React from "react";
import { ServerStyleContext, ClientStyleContext } from "~/context";

const Document = withEmotionCache(
  ({ children }: React.PropsWithChildren, emotionCache) => {
    const serverStyleData = React.useContext(ServerStyleContext);
    const clientStyleData = React.useContext(ClientStyleContext);

    React.useEffect(
      () => {
        emotionCache.sheet.container = document.head;
        const tags = emotionCache.sheet.tags;
        emotionCache.sheet.flush();
        tags.forEach((tag) => {
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          (emotionCache.sheet as any)._insertTag(tag);
        });
        clientStyleData?.reset();
      },
      // eslint-disable-next-line react-hooks/exhaustive-deps
      [],
    );

    return (
      <html lang="en">
        <head>
          <Meta />
          <Links />
          {serverStyleData?.map(({ key, ids, css }) => (
            <style
              key={key}
              data-emotion={`${key} ${ids.join(' ')}`}
              dangerouslySetInnerHTML={{ __html: css }}
            />
          ))}
        </head>
        <body>
          {children}
          <ScrollRestoration />
          <Scripts />
          <LiveReload />
        </body>
      </html>
    );
  }
);

export default function App() {
  return (
    <Document>
      <ChakraProvider>
        <Outlet />
      </ChakraProvider>
    </Document>
  );
}
