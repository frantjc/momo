import * as core from "@actions/core";
import { Octokit } from "octokit";

const package_type = "container";

async function run(): Promise<void> {
  try {
    const octokit = new Octokit({
      auth: core.getInput("token", {
        required: true,
      }),
    });

    const tags = core.getMultilineInput("tags", {
      required: true,
    });

    for (const tag of tags) {
      const cparts = tag.split(":");

      if (cparts.length == 2) {
        const sparts = cparts[0].split("/");
        if (sparts.length < 3) {
          throw new Error(`invalid tag ${tag}`);
        }

        const [registry, username, ...rest] = sparts[0];
        if (registry !== "ghcr.io") {
          throw new Error("tags must refer to ghcr.io");
        }

        const package_name = rest.join("/");

        const _tag = cparts[1];

        if (!username || !package_name || !_tag) {
          throw new Error(`invalid tag ${tag}`);
        }

        const packageVersions =
          await octokit.rest.packages.getAllPackageVersionsForPackageOwnedByUser(
            {
              state: "active",
              package_type,
              package_name,
              username,
            },
          );

        const packageVersion = packageVersions.data.find((packageVersion) => {
          return packageVersion.metadata?.container?.tags.includes(_tag);
        });

        if (packageVersion) {
          if (packageVersions.data.length === 1) {
            await octokit.rest.packages.deletePackageForUser({
              package_type,
              package_name,
              username,
            });
          } else {
            await octokit.rest.packages.deletePackageVersionForUser({
              package_type,
              package_name,
              username,
              package_version_id: packageVersion.id,
            });
          }
        } else {
          throw new Error("unable to find package version ID");
        }
      } else {
        throw new Error(`invalid tag ${tag}`);
      }
    }
  } catch (err) {
    if (typeof err === "string" || err instanceof Error) {
      core.setFailed(err);
    } else {
      core.setFailed("caught unknown error");
    }
  }
}

run();
