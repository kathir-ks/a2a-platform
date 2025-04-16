import os
import argparse
from typing import List

def create_prompt_from_directory(directory_path: str, exclude_dirs: List[str] = None) -> str:
    """
    Recursively walks through a directory, reads the content of all files,
    and concatenates them into a single string with placeholders.

    Args:
        directory_path: The path to the target directory.
        exclude_dirs: A list of directory names to exclude from the scan.
                      These are matched against the *basename* of the directories.

    Returns:
        A single string containing the concatenated content of all files found,
        separated by placeholders indicating file boundaries.
        Returns an empty string if the directory is invalid or inaccessible.
    """
    if not os.path.isdir(directory_path):
        print(f"Error: Provided path '{directory_path}' is not a valid directory.")
        return ""

    prompt_parts = []
    separator_start = "\n--- START OF FILE: {filepath} ---\n"
    separator_end = "\n--- END OF FILE: {filepath} ---\n"

    print(f"Scanning directory: {directory_path}")

    if exclude_dirs is None:
        exclude_dirs = [] # Initialize empty list to avoid NoneType errors later

    for root, dirs, files in os.walk(directory_path):
        #  Mutate the dirs list in-place to prevent os.walk from entering excluded directories
        dirs[:] = [d for d in dirs if d not in exclude_dirs]
        files.sort()  #Sort files for more predicatble output
        dirs.sort()  #Sort directories as well

        print(f"  Entering: {root}") # Progress indicator

        for filename in files:
            file_path = os.path.join(root, filename)
            relative_path = os.path.relpath(file_path, directory_path) # Get path relative to start dir

            print(f"    Reading: {relative_path}") # Progress indicator

            prompt_parts.append(separator_start.format(filepath=relative_path))
            try:
                # Use 'with' to ensure file is closed automatically
                # Specify encoding, utf-8 is common. Handle potential errors.
                with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                    content = f.read()
                    prompt_parts.append(content)
            except Exception as e:
                # Catch potential OS errors (permissions) or other read issues
                error_message = f"\n*** Could not read file: {relative_path} | Error: {e} ***\n"
                print(f"    Warning: {error_message.strip()}")
                prompt_parts.append(error_message) # Add error message to prompt

            prompt_parts.append(separator_end.format(filepath=relative_path))

    print("Finished scanning.")
    return "".join(prompt_parts)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Recursively read files in a directory and concatenate their content into a single prompt."
    )
    parser.add_argument(
        "directory",
        help="The path to the directory to scan."
    )
    parser.add_argument(
        "-e", "--exclude",
        nargs='+', #Allows multiple values
        help="List of directory names to exclude (base name only).",
        default=[]
    )
    parser.add_argument(
        "-o", "--output",
        help="Optional: Path to a file where the resulting prompt should be saved.",
        default=None
    )

    args = parser.parse_args()

    target_directory = args.directory
    exclude_directories = args.exclude  #List of excluded directories

    # --- Perform the operation ---
    full_prompt = create_prompt_from_directory(target_directory, exclude_directories)

    if full_prompt:
        if args.output:
            try:
                with open(args.output, 'w', encoding='utf-8') as outfile:
                    outfile.write(full_prompt)
                print(f"\nSuccessfully saved prompt to: {args.output}")
            except Exception as e:
                print(f"\nError writing prompt to file '{args.output}': {e}")
        else:
            # --- Print the result to the console ---
            print("\n--- Generated Prompt ---")
            print(full_prompt)
            print("--- End of Prompt ---")
            print(f"\nTotal length of prompt: {len(full_prompt)} characters")

    else:
        print("No prompt generated (invalid directory or empty).")