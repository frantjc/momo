import { unstable_useEnhancedEffect as useEnhancedEffect } from "@mui/material";
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
    const reinjectStylesRef = React.useRef(true);

    useEnhancedEffect(() => {
      if (!reinjectStylesRef.current) {
        return;
      }

      emotionCache.sheet.container = document.head;

      const tags = emotionCache.sheet.tags;
      emotionCache.sheet.flush();
      tags.forEach((tag) => {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        (emotionCache.sheet as any)._insertTag(tag);
      });

      clientStyleData?.reset();
      reinjectStylesRef.current = false;
    }, [reinjectStylesRef, clientStyleData, emotionCache.sheet]);

    return (
      <html lang="en">
        <head>
          <Meta />
          <Links />
          {serverStyleData?.map(({ key, ids, css }) => (
            <style
              key={key}
              data-emotion={`${key} ${ids.join(" ")}`}
              // eslint-disable-next-line react/no-danger
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
  },
);

export default function App() {
  return (
    <Document>
      <Outlet />
    </Document>
  );
}
