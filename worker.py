#!/usr/bin/env python3
"""
DroidRun worker - reads task from stdin, runs agent, outputs result to stdout
"""
import sys
import json
import asyncio
import subprocess
import time


def adb_go_home():
    """Press the home button via ADB to return to the home screen."""
    try:
        subprocess.run(
            ["adb", "shell", "input", "keyevent", "KEYCODE_HOME"],
            capture_output=True, timeout=10,
        )
    except Exception as e:
        print(f"[worker] adb go home failed: {e}", file=sys.stderr)


def adb_launch_app(package: str):
    """Launch an app by package name via ADB."""
    try:
        subprocess.run(
            ["adb", "shell", "monkey", "-p", package,
             "-c", "android.intent.category.LAUNCHER", "1"],
            capture_output=True, timeout=10,
        )
        time.sleep(2)  # Wait for app to start
    except Exception as e:
        print(f"[worker] adb launch {package} failed: {e}", file=sys.stderr)


def adb_open_deeplink(uri: str):
    """Open a deep link URI via ADB (using VIEW intent)."""
    try:
        subprocess.run(
            ["adb", "shell", "am", "start", "-a",
             "android.intent.action.VIEW", "-d", uri],
            capture_output=True, timeout=10,
        )
        time.sleep(2)  # Wait for deep link to resolve
    except Exception as e:
        print(f"[worker] adb open deeplink {uri} failed: {e}", file=sys.stderr)


def create_llm(provider: str, model: str, api_key: str = None):
    """Create LLM instance based on provider"""

    if provider in ("Google", "GoogleGenAI", "Gemini"):
        from llama_index.llms.gemini import Gemini
        # Gemini models need "models/" prefix
        if not model.startswith("models/"):
            model = f"models/{model}"
        return Gemini(model=model, api_key=api_key)

    elif provider == "Anthropic":
        from llama_index.llms.anthropic import Anthropic
        return Anthropic(model=model, api_key=api_key)

    elif provider == "OpenAI":
        from llama_index.llms.openai import OpenAI
        return OpenAI(model=model, api_key=api_key)

    elif provider == "DeepSeek":
        from llama_index.llms.deepseek import DeepSeek
        return DeepSeek(model=model, api_key=api_key)

    elif provider == "Ollama":
        from llama_index.llms.ollama import Ollama
        return Ollama(model=model)

    else:
        raise ValueError(f"Unknown provider: {provider}")


async def run_task(task: dict) -> dict:
    from droidrun import DroidAgent, DroidrunConfig, AgentConfig

    api_key = task.get("api_key")
    if not api_key:
        raise ValueError("api_key is required")

    llm = create_llm(task["provider"], task["model"], api_key)

    config = DroidrunConfig(
        agent=AgentConfig(
            reasoning=task.get("reasoning", True),
            max_steps=task.get("max_steps", 30),
            streaming=False,  # Disable streaming to avoid llama-index async generator bug
        )
    )

    agent = DroidAgent(
        goal=task["goal"],
        config=config,
        llms=llm,  # Single LLM for all agents
    )

    result = await agent.run()

    return {
        "success": result.success,
        "reason": result.reason,
        "steps": result.steps if hasattr(result, 'steps') else None,
    }


def main():
    task = json.load(sys.stdin)

    # Redirect stdout to stderr during execution (droidrun prints thoughts)
    real_stdout = sys.stdout
    sys.stdout = sys.stderr

    # Launch app and/or open deep link via ADB (deterministic, doesn't depend on LLM)
    app = task.get("app")
    deeplink = task.get("deeplink")
    if app:
        adb_launch_app(app)
    if deeplink:
        adb_open_deeplink(deeplink)

    try:
        result = asyncio.run(run_task(task))
        # Restore stdout for final JSON output
        sys.stdout = real_stdout
        print(json.dumps({"ok": True, **result}))
    except Exception as e:
        sys.stdout = real_stdout
        print(json.dumps({"ok": False, "error": str(e)}))
    finally:
        # Always return to home screen when task ends
        adb_go_home()


if __name__ == "__main__":
    main()
