import { CacheProvider } from "@emotion/react"
import { RemixBrowser } from "@remix-run/react";
import React from "react";
import { hydrateRoot } from "react-dom/client";

import { ClientStyleContext } from "~/context";
import createEmotionCache, { defaultCache } from "~/create_emotion_cache";
import { Themed } from "~/theme";

interface ClientCacheProviderProps {
  children: React.ReactNode;
}

function ClientCacheProvider({ children }: ClientCacheProviderProps) {
  const [cache, setCache] = React.useState(defaultCache)

  const reset = React.useCallback(() => {
    setCache(createEmotionCache());
  }, [setCache]);

  return (
    <ClientStyleContext.Provider value={{ reset }}>  
      <CacheProvider value={cache}><Themed>{children}</Themed></CacheProvider>
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
