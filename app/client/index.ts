import path from "path";

export type App = {
    id: string
    name: string;
    version?: string;
    status: string;
    bundleName?: string;
    bundleIdentifier?: string;
    sha256CertFingerprints?: string;
    created: Date
    updated: Date
}

export function getApps() {
    return fetch(getUrl("/api/v1/apps"))
        .then(handleError)
        .then((res) => {
          return res.json() as Promise<Array<App>>;
        })
        .then(apps => apps.map(typeApp))
}

export function getApp(app: Pick<App, "id"> | Pick<App, "name" | "version">) {
  if ("id" in app && app.id) {
    return fetch(getUrl(path.join("/api/v1/apps", app.id)))
        .then(handleError)
        .then((res) => {
          return res.json() as Promise<App>;
        })
        .then(typeApp);
  } else if ("name" in app) {
    return fetch(getUrl(path.join("/api/v1/apps", app.name, app.version || "")))
        .then(handleError)
        .then((res) => {
          return res.json() as Promise<App>;
        })
        .then(typeApp);
  }

  throw new Error("unable to uniquely identify app");
}

function typeApp(app: App) {
  return { ...app, updated: new Date(app.updated), created: new Date(app.created) };
}

// getUrl takes a path and returns the full URL
// that that resource can be accessed at. This
// cleverly works both in the browser and in NodeJS.
function getUrl(path: string) {
    if (typeof process !== "object") {
      return path;
    } else if (process.env.MOMO_API_URL) {
      return new URL(path, process.env.MOMO_API_URL).toString();
    }
  
    return new URL(
      path,
      `http://localhost:${process.env.PORT || 3000}`,
    ).toString();
  }

function isSuccess(res: Response) {
    return 200 <= res.status && res.status < 300;
  }
  
  function isError(res: Response) {
    return !isSuccess(res);
  }
  
  function handleError(res: Response) {
    if (isError(res)) {
      return res
        .json()
        .catch(() => {
          throw new Response(null, {
            status: res.status,
            statusText: res.statusText,
          });
        })
        .then((err) => {
          // Errors from the API look like '{"error":"error description"}'.
          throw new Response(null, {
            status: res.status,
            statusText: err.error || res.statusText,
          });
        });
    }
  
    return res;
  }
