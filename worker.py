#!/usr/bin/env python3
"""
DroidRun worker - reads task from stdin, runs agent, outputs result to stdout
"""
import sys
import json
import asyncio


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

    try:
        result = asyncio.run(run_task(task))
        # Restore stdout for final JSON output
        sys.stdout = real_stdout
        print(json.dumps({"ok": True, **result}))
    except Exception as e:
        sys.stdout = real_stdout
        print(json.dumps({"ok": False, "error": str(e)}))


if __name__ == "__main__":
    main()
