import * as core from "@actions/core";
import { Octokit } from "octokit";

async function run(): Promise<void> {
  try {
    throw new Error("TODO");

    const octokit = new Octokit({});

    const res = await octokit.rest.packages.deletePackageForOrg({
      package_type: "container",
      package_name: "momo",
      org: "frantjc",
      package_version_id: 0, // TODO.
    });
  } catch (err) {
    if (typeof err === "string" || err instanceof Error) core.setFailed(err);
  }
}

run();
