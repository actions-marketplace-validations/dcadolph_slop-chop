package com.slopchop.jetbrains

import com.intellij.execution.configurations.GeneralCommandLine
import com.intellij.openapi.project.Project
import com.redhat.devtools.lsp4ij.LanguageServerFactory
import com.redhat.devtools.lsp4ij.server.OSProcessStreamConnectionProvider
import com.redhat.devtools.lsp4ij.server.StreamConnectionProvider

/** SERVER_ID is the id plugin.xml registers the slop-chop server under. */
const val SERVER_ID = "slopChop"

/**
 * SlopChopLanguageServerFactory registers the slop-chop LSP server with LSP4IJ. The server is
 * the local slop-chop binary run as `slop-chop lsp`, so the same rules engine that powers the
 * CLI, the web app, and the other editors backs JetBrains too.
 */
class SlopChopLanguageServerFactory : LanguageServerFactory {
    override fun createConnectionProvider(project: Project): StreamConnectionProvider {
        val settings = SlopChopSettings.getInstance().state
        return SlopChopConnectionProvider(commandLine(project.basePath, settings.codeComments))
    }
}

/**
 * commandLine builds the `slop-chop lsp` command for a project. It runs in the project folder,
 * so the binary finds the project's .slop-chop.json the way it does from a terminal there, and
 * passes --code-comments=false when comment checking is off.
 */
fun commandLine(workDir: String?, codeComments: Boolean): GeneralCommandLine {
    val cmd = GeneralCommandLine("slop-chop", "lsp")
    if (!codeComments) cmd.addParameter("--code-comments=false")
    if (workDir != null) cmd.withWorkDirectory(workDir)
    return cmd
}

/**
 * SlopChopConnectionProvider launches the slop-chop language server and speaks LSP over its
 * stdio. The binary must be on the PATH; a missing binary surfaces as a server start error in
 * the LSP4IJ console.
 */
class SlopChopConnectionProvider(commandLine: GeneralCommandLine) :
    OSProcessStreamConnectionProvider(commandLine)
