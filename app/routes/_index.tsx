import { Container } from "@mui/material";
import type { MetaFunction } from "@remix-run/node";
import { AppUploadForm } from "~/components";

export const meta: MetaFunction = () => {
  return [
    { title: "New Remix App" },
    { name: "description", content: "Welcome to Remix!" },
  ];
};

export default function Index() {
  return (
    <Container>
      <AppUploadForm />
    </Container>
  );
}
