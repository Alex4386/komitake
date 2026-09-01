import React from "react";
import ReactDOM from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import i18n from "./i18n";
import "./index.css";

function syncDocumentLanguage(language: string) {
  document.documentElement.lang = language.split("-")[0];
}

syncDocumentLanguage(i18n.language);
i18n.on("languageChanged", syncDocumentLanguage);

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider attribute="class" defaultTheme="dark" enableSystem disableTransitionOnChange>
      <BrowserRouter basename="/ui">
        <App />
      </BrowserRouter>
    </ThemeProvider>
  </React.StrictMode>
);
