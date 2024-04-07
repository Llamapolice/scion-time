import json
import subprocess
import os
import datetime
import argparse


def read_lines(file_path_1, file_path_2):
    with open(file_path_1, 'r') as file_1, open(file_path_2, 'r') as file_2:
        lines_file_1 = [remove_time_from_json(line) for line in file_1.readlines()]
        lines_file_2 = [remove_time_from_json(line) for line in file_2.readlines()]

    split_lines_file_1 = sort_individual_time_steps(lines_file_1)
    split_lines_file_2 = sort_individual_time_steps(lines_file_2)

    with open('logs/result_new.txt', 'w') as file:
        for line in split_lines_file_1:
            file.write(line + '\n')
    with open('logs/result_old.txt', 'w') as file:
        for line in split_lines_file_2:
            file.write(line + '\n')

    return split_lines_file_1, split_lines_file_2


def remove_time_from_json(json_line):
    json_object = json.loads(json_line)
    if 'T' in json_object:
        del json_object['T']
    return json.dumps(json_object)


def sort_individual_time_steps(lines):
    result = []
    current_sublist = []
    header = ""
    for line in lines:
        if 'Handling the next' in line:
            if current_sublist:
                result.append(header)
                current_sublist.sort()
                result += current_sublist
                current_sublist = []
                header = line
                continue
        current_sublist.append(line)
    if current_sublist:
        current_sublist.sort()
        result += current_sublist
    return result


def compare_two_logs(first, second):
    lines_file_1, lines_file_2 = read_lines(first, second)
    if len(lines_file_1) != len(lines_file_2):
        raise ValueError("The number of lines in the two files is not equal. "
                         "First file: " + first + " Second file: " + second)
    for i in range(len(lines_file_1)):
        if lines_file_1[i] != lines_file_2[i]:
            raise ValueError("The lines at index {} are not equal. "
                             "First file: ".format(i) + first + " Second file: " + second)
    print("Files are equal")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Small script to evaluate reproducibility of the G-SINC simulator.'
                                                 'The script runs the simulation with the config provided at '
                                                 'config_path num_tests times and compares their output.'
                                                 'If one output is not equivalent to the previous ones, '
                                                 'the script fails. '
                                                 'Raw output from the simulation is saved to logs/[datetime]/ '
                                                 'and the two most recent processed logs are stored in '
                                                 'logs/result_*.txt.')

    parser.add_argument('-n',
                        '--num_tests',
                        type=int,
                        default=10,
                        help='The number of tests to run. Default is 10.')

    parser.add_argument('-c',
                        '--config_path',
                        type=str,
                        default='simulation/configs/simulation_test.toml',
                        help='Path to the configuration file relative to timeservice executable. Default path is '
                             '"simulation/configs/simulation_test.toml".')
    args = parser.parse_args()
    num_tests = args.num_tests
    config_path = args.config_path

    timestamp = datetime.datetime.now().strftime('%Y-%m-%d_%H-%M-%S')
    dir_path = os.path.join('logs/', timestamp)
    os.makedirs(dir_path, exist_ok=True)

    log_folder = os.path.join(os.getcwd(), dir_path)
    log = os.path.join(log_folder, '0.log')
    prev_log = log

    print("Running " + log)
    command = ["./timeservice", "simulation", "-config", config_path, "-console=false", "-logfile"]
    subprocess.run(command + [log], cwd="../../")

    for i in range(1, num_tests):
        log = os.path.join(log_folder, str(i) + '.log')
        print("Running " + log)
        subprocess.run(command + [log], cwd="../../")
        compare_two_logs(log, prev_log)
        prev_log = log

    print("Finished running")
