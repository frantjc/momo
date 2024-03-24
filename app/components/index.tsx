import { styled, StyledComponentProps } from "@mui/material";
import path from "path";
import React from "react";
import { App } from "~/client";

const Form = styled("form")({});
const Input = styled("input")({});
const Label = styled("label")({});

type UploadableApp = Pick<App, "name" | "version"> & { id?: string };

export type AppUploadFormProps = Omit<StyledComponentProps<"form">, "action" | "method" | "encType"> & { app?: UploadableApp };

export function AppUploadForm(props: AppUploadFormProps) {
    const [app, setApp] = React.useState<UploadableApp | undefined>(props.app);

    return (
      <Form
        {...(app?.id
          ? {
            action: path.join("/api/v1/apps", app?.id || ""),
            method: "put",
          } : {
            action: path.join("/api/v1/apps", app?.name || "", app?.version || ""),
            method: "post",
          }
        )}
        encType="multipart/form-data"
        {...props}
      >
        <Label htmlFor="name">Name</Label>
        <Input name="name" type="text" placeholder="my-app" required onChange={(event) => {
          event.preventDefault();
          setApp((_app) => ({ ..._app, name: event.target.value }));
        }} />
        <Label htmlFor="version">Version</Label>
        <Input name="version" type="text" placeholder="v1.0.0" onChange={(event) => {
          event.preventDefault();
          setApp((_app) => ({ ..._app, version: event.target.value }));
        }}/>
        <Label htmlFor="files">Files</Label>
        <Input name="files" type="file" required multiple />
        <Input
          type="submit"
          value="Upload"
          disabled={!app?.name}
          accept=".ipa, .apk, .png"
        />
      </Form>
    );
}
