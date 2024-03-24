import {
    CssBaseline,
    ThemeProvider,
    createTheme,
    responsiveFontSizes,
} from "@mui/material";

const theme = responsiveFontSizes(createTheme());

export function Themed({ children }: React.PropsWithChildren) {
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            {children}
        </ThemeProvider>
    );
}
  