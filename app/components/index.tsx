import { Box, FormControl, FormLabel, Input } from "@chakra-ui/react";
import path from "path";
import React from "react";
import { App } from "~/client";

export function AppUploadForm() {
    const [app, setApp] = React.useState<Pick<App, "name" | "version">>();

    return (
      <Box>
        <FormControl isRequired>
          <FormLabel>Name</FormLabel>
          <Input type="text" placeholder="my-app" onChange={(event) => {
            setApp((_app) => ({ ..._app, name: event.target.value }));
          }} />
        </FormControl>
        <FormControl>
          <FormLabel>Version</FormLabel>
          <Input type="text" placeholder="v1.0.0" onChange={(event) => {
            setApp((_app) => ({ ..._app, version: event.target.value }));
          }}/>
        </FormControl>
        <form action={path.join("/api/v1/apps", app?.name || "", app?.version || "")} method="post" encType="multipart/form-data">
          <Input type="file" name="file" required multiple />
          <Input
            type="submit"
            value="Upload"
            isDisabled={!app?.name}
            accept=".ipa,.apk,.png"
          />
        </form>
      </Box>
    );
}
