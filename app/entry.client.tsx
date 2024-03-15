import { CacheProvider } from "@emotion/react"
import { RemixBrowser } from "@remix-run/react";
import React from "react";
import { hydrateRoot } from "react-dom/client";

import { ClientStyleContext } from "~/context";
import createEmotionCache, { defaultCache } from "~/create_emotion_cache";

interface ClientCacheProviderProps {
  children: React.ReactNode;
}

function ClientCacheProvider({ children }: ClientCacheProviderProps) {
  const [cache, setCache] = React.useState(defaultCache)

  function reset() {
    setCache(createEmotionCache())
  }

  return (
    <ClientStyleContext.Provider value={{ reset }}>
      <CacheProvider value={cache}>{children}</CacheProvider>
    </ClientStyleContext.Provider>
  )
}

React.startTransition(() => {
  hydrateRoot(
    document,
    <React.StrictMode>
      <ClientCacheProvider>
          <RemixBrowser />
      </ClientCacheProvider>
    </React.StrictMode>
  );
});
