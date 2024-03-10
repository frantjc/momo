import * as core from "@actions/core";
import { Octokit } from "octokit";

async function run(): Promise<void> {
  try {
    const octokit = new Octokit({
      auth: process.env.GITHUB_TOKEN,
    });

    const packageVersions = await octokit.rest.packages.getAllPackageVersionsForPackageOwnedByUser({
      state: "active",
      package_type: "container",
      package_name: "momo",
      username: "frantjc",
    });

    const githubSha = process.env.GITHUB_SHA;
    if (githubSha) {
      const packageVersion = packageVersions.data.find(packageVersion => {
        return packageVersion.metadata?.container?.tags.includes(githubSha);
      });

      if (packageVersion) {
        await octokit.rest.packages.deletePackageVersionForUser({
          package_type: "docker",
          package_name: "momo",
          username: "frantjc",
          package_version_id: packageVersion?.id,
        });
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
