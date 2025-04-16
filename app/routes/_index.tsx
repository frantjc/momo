import type { MetaFunction } from "@remix-run/node";

export const meta: MetaFunction = () => {
  return [
    { title: "Momo" },
    { name: "description", content: "Distribute enterprise mobile applications." },
  ];
};

export default function Index() {
  return (
    <div />
  );
}
