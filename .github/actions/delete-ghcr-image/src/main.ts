import * as core from "@actions/core";
import { Octokit } from "octokit";

async function run(): Promise<void> {
  try {
    const octokit = new Octokit({
      auth: process.env.GITHUB_TOKEN,
    });

    const res = await octokit.rest.packages.deletePackageForOrg({
      package_type: "docker",
      package_name: "momo",
      org: "frantjc",
      package_version_id: 0, // TODO.
    });
  } catch (err) {
    if (typeof err === "string" || err instanceof Error) {
      core.setFailed(err);
    } else {
      core.setFailed("caught unknown error");
    }
  }
}

run();
