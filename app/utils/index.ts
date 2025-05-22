import { MetaFunction } from "@remix-run/react";

export const meta: MetaFunction = () => {
  const title = "Momo";
  const description = "Distribute enterprise mobile applications to non-managed devices.";

  let url;
  try {
    if (typeof process !== "object") {
      url = new URL(window.location.href);
    } else {
      const port = process.env.PORT || 3000;
      const base = process.env.URL || `http://localhost:${port}/`;
      url = location && new URL(location.pathname, base);
    }
  } catch (_) {
    //nolint
  }

  return [
    { charSet: "utf-8" },
    { name: "viewport", content: "width=device-width,initial-scale=1" },
    { property: "og:site_name", content: title },
    { title },
    { property: "og:title", content: title },
    { property: "twitter:title", content: title },
    { name: "description", content: description },
    { property: "og:description", content: description },
    { property: "twitter:description", content: description },
    { property: "og:type", content: "website" },
    { property: "twitter:card", content: "summary" },
    ...((url && [
      { property: "og:url", content: url.toString() },
      { property: "twitter:domain", content: url.hostname },
      { property: "twitter:url", content: url.toString() },
    ]) ||
      []),
  ];
};
